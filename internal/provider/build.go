package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

	"chainguard.dev/apko/pkg/build"
	"chainguard.dev/apko/pkg/build/oci"
	"chainguard.dev/apko/pkg/build/types"
	"chainguard.dev/apko/pkg/options"
	"github.com/chainguard-dev/clog"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	coci "github.com/sigstore/cosign/v2/pkg/oci"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/util/sets"
)

func fromImageData(ctx context.Context, ic types.ImageConfiguration, popts ProviderOpts) (*options.Options, *types.ImageConfiguration, error) {
	// Deduplicate any of the extra packages against their potentially resolved
	// form in the actual image list.
	pkgs := sets.New(ic.Contents.Packages...)
	extraPkgs := sets.New(popts.packages...)
	for _, pkg := range sets.List(pkgs) {
		name := pkg
		// The function we want from go-apk is private, but these are all the
		// special characters that delimit the package name from the constraint
		// so lop off the package name and stick the rest of the constraint into
		// the versions map.
		if idx := strings.IndexAny(pkg, "=<>~"); idx >= 0 {
			name = pkg[:idx]
		}
		extraPkgs.Delete(name)
	}
	ic.Contents.Packages = sets.List(pkgs.Union(extraPkgs))

	// Normalize the architecture by calling ParseArchitecture.  This is
	// something sublte that `apko` gets for free because it only accepts yaml
	// and the yaml parsing normalizes things.
	for i, arch := range ic.Archs {
		ic.Archs[i] = types.ParseArchitecture(arch.String())
	}

	opts := []build.Option{
		build.WithImageConfiguration(ic),
		build.WithSBOMFormats([]string{"spdx"}),
		build.WithExtraKeys(popts.keyring),
		build.WithExtraRuntimeRepos(popts.repositories),
		build.WithExtraBuildRepos(popts.buildRespositories),
	}

	o, ic2, err := build.NewOptions(opts...)
	if err != nil {
		return nil, nil, err
	}

	if len(ic2.Archs) != 0 {
		// If the configuration has architectures, use them.
	} else if len(popts.archs) != 0 {
		// Otherwise, fallback on the provider architectures.
		ic2.Archs = types.ParseArchitectures(popts.archs)
	} else {
		// If neither is specified, build for all architectures!
		ic2.Archs = types.AllArchs
	}

	return o, ic2, nil
}

type imagesbom struct {
	imageHash       v1.Hash
	predicateType   string
	predicatePath   string
	predicateSHA256 string
}

func doBuild(ctx context.Context, data BuildResourceModel) (v1.Hash, coci.SignedEntity, map[string]imagesbom, error) {
	tempDir, err := os.MkdirTemp("", "apko-*")
	if err != nil {
		return v1.Hash{}, nil, nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	var ic types.ImageConfiguration
	if diags := assignValue(data.Config, &ic); diags.HasError() {
		return v1.Hash{}, nil, nil, fmt.Errorf("assigning value: %v", diags.Errors())
	}

	tflog.Trace(ctx, fmt.Sprintf("Got image configuration: %#v", ic))

	// Parse things once to determine the architectures to build from
	// the config.
	o, ic2, err := fromImageData(ctx, ic, data.popts)
	if err != nil {
		return v1.Hash{}, nil, nil, err
	}

	// We compute the "build date epoch" of the multi-arch image to be the
	// maximum "build date epoch" of the per-arch images.  If the user has
	// explicitly set SOURCE_DATE_EPOCH, that will always trump this
	// computation.
	multiArchBDE := o.SourceDateEpoch

	var mu sync.Mutex
	imgs := make(map[types.Architecture]coci.SignedImage, len(ic2.Archs))
	contexts := make(map[types.Architecture]*build.Context, len(ic2.Archs))
	sboms := make(map[string]imagesbom, len(ic2.Archs)+1)

	mc, err := build.NewMultiArch(ctx, ic2.Archs, build.WithImageConfiguration(*ic2),
		build.WithSBOMFormats([]string{"spdx"}),
		build.WithSBOM(tempDir),
		build.WithExtraKeys(data.popts.keyring),
		build.WithExtraBuildRepos(data.popts.buildRespositories),
		build.WithExtraRuntimeRepos(data.popts.repositories))
	if err != nil {
		return v1.Hash{}, nil, nil, err
	}

	layers, err := mc.BuildLayers(ctx)
	if err != nil {
		return v1.Hash{}, nil, nil, fmt.Errorf("building layers: %w", err)
	}

	var errg errgroup.Group
	for _, arch := range ic2.Archs {
		arch := arch

		log := clog.New(slog.Default().Handler()).With("arch", arch.ToAPK())
		ctx := clog.WithLogger(ctx, log)

		errg.Go(func() error {
			bc := mc.Contexts[arch]
			layer := layers[arch]

			bde, err := bc.GetBuildDateEpoch()
			if err != nil {
				return fmt.Errorf("failed to determine build date epoch: %w", err)
			}

			img, err := oci.BuildImageFromLayer(ctx, empty.Image, layer, bc.ImageConfiguration(), bde, bc.Arch())
			if err != nil {
				return fmt.Errorf("failed to build OCI image for %q: %w", arch, err)
			}

			outputs, err := bc.GenerateImageSBOM(ctx, arch, img)
			if err != nil {
				return fmt.Errorf("generating sbom for %s: %w", arch, err)
			}

			h, err := img.Digest()
			if err != nil {
				return fmt.Errorf("unable to compute digest for %q: %w", arch, err)
			}

			// We have hardcoded sbom formats to be just "spdx", fail if this isn't right.
			if len(outputs) != 1 {
				return fmt.Errorf("saw %d sbom outputs, expected 1", len(outputs))
			}

			// Move the sbom to a temporary file outside of the directory we
			// plan to clean up, so that it outlives the evaluation of this
			// build resource.
			sbomPath := outputs[0].Path
			f, err := os.CreateTemp("", "sbom-*.spdx.json")
			if err != nil {
				return fmt.Errorf("unable to create temporary file for sbom: %w", err)
			}
			defer f.Close()

			content, err := os.ReadFile(sbomPath)
			if err != nil {
				return fmt.Errorf("unable to read SBOM %q: %w", arch, err)
			}
			if _, err := f.Write(content); err != nil {
				return fmt.Errorf("failed to write sbom to %q: %w", f.Name(), err)
			}
			hash := sha256.Sum256(content)

			mu.Lock()
			defer mu.Unlock()

			// Adjust the index's builder to track the most recent BDE.
			if bde.After(multiArchBDE) {
				multiArchBDE = bde
			}

			// save the build context for later
			contexts[arch] = bc
			imgs[arch] = img

			sboms[arch.String()] = imagesbom{
				imageHash:       h,
				predicateType:   "https://spdx.dev/Document",
				predicatePath:   f.Name(),
				predicateSHA256: hex.EncodeToString(hash[:]),
			}

			return nil
		})
	}

	if err := errg.Wait(); err != nil {
		return v1.Hash{}, nil, nil, err
	}

	// generate the index
	finalDigest, idx, err := oci.GenerateIndex(ctx, *ic2, imgs)
	if err != nil {
		return v1.Hash{}, nil, nil, fmt.Errorf("failed to generate OCI index: %w", err)
	}

	o, ic2, err = build.NewOptions(
		build.WithImageConfiguration(*ic2),      // We mutate Archs above.
		build.WithSourceDateEpoch(multiArchBDE), // Maximum child's time.
		build.WithSBOMFormats([]string{"spdx"}),
		build.WithSBOM(tempDir),
		build.WithExtraKeys(data.popts.keyring),
		build.WithExtraRuntimeRepos(data.popts.repositories),
		build.WithExtraBuildRepos(data.popts.buildRespositories),
	)
	if err != nil {
		return v1.Hash{}, nil, nil, fmt.Errorf("failed to create options for index: %w", err)
	}

	isboms, err := build.GenerateIndexSBOM(ctx, *o, *ic2, finalDigest, imgs)
	if err != nil {
		return v1.Hash{}, nil, nil, fmt.Errorf("generating index SBOM: %w", err)
	}

	// Move the sbom to a temporary file outside of the directory we
	// plan to clean up, so that it outlives the evaluation of this
	// build resource.
	sbomPath := isboms[0].Path
	f, err := os.CreateTemp("", "sbom-*.spdx.json")
	if err != nil {
		return v1.Hash{}, nil, nil, fmt.Errorf("unable to create temporary file for sbom: %w", err)
	}
	defer f.Close()
	content, err := os.ReadFile(sbomPath)
	if err != nil {
		return v1.Hash{}, nil, nil, fmt.Errorf("unable to read index SBOM: %w", err)
	}
	if _, err := f.Write(content); err != nil {
		return v1.Hash{}, nil, nil, fmt.Errorf("failed to write sbom to %q: %w", f.Name(), err)
	}
	hash := sha256.Sum256(content)

	h, err := idx.Digest()
	if err != nil {
		return v1.Hash{}, nil, nil, fmt.Errorf("unable to compute digest for index: %w", err)
	}

	sboms["index"] = imagesbom{
		imageHash:       h,
		predicateType:   "https://spdx.dev/Document",
		predicatePath:   f.Name(),
		predicateSHA256: hex.EncodeToString(hash[:]),
	}
	return h, idx, sboms, nil
}

func remoteBuild(ctx context.Context, data BuildResourceModel) (string, error) {
	u := fmt.Sprintf("%s/build?repo=%s", *data.popts.remoteBuilder, data.Repo.ValueString())
	var ic types.ImageConfiguration
	if diags := assignValue(data.Config, &ic); diags.HasError() {
		return "", fmt.Errorf("assigning value: %v", diags.Errors())
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(ic); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	hresp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer hresp.Body.Close()
	all, err := io.ReadAll(hresp.Body)
	if err != nil {
		return "", err
	}
	if hresp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("remote build failed: %s", string(all))
	}
	return string(all), nil
}
