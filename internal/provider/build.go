package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"chainguard.dev/apko/pkg/build"
	"chainguard.dev/apko/pkg/build/oci"
	"chainguard.dev/apko/pkg/build/types"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	ggcrtypes "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	coci "github.com/sigstore/cosign/v2/pkg/oci"
	ocimutate "github.com/sigstore/cosign/v2/pkg/oci/mutate"
	"github.com/sigstore/cosign/v2/pkg/oci/signed"
	"golang.org/x/sync/errgroup"
)

func fromImageData(ic types.ImageConfiguration, popts ProviderOpts, wd string) (*build.Context, error) {
	ic.Contents.Packages = append(ic.Contents.Packages, popts.packages...)

	// Normalize the architecture by calling ParseArchitecture.  This is
	// something sublte that `apko` gets for free because it only accepts yaml
	// and the yaml parsing normalizes things.
	for i, arch := range ic.Archs {
		ic.Archs[i] = types.ParseArchitecture(arch.String())
	}

	bc, err := build.New(wd,
		build.WithImageConfiguration(ic),
		build.WithSBOMFormats([]string{"spdx"}),
		build.WithExtraKeys(popts.keyring),
		build.WithExtraRepos(popts.repositories),
	)

	if err != nil {
		return nil, err
	}

	if len(bc.ImageConfiguration.Archs) != 0 {
		// If the configuration has architectures, use them.
	} else if len(popts.archs) != 0 {
		// Otherwise, fallback on the provider architectures.
		bc.ImageConfiguration.Archs = types.ParseArchitectures(popts.archs)
	} else {
		// If neither is specified, build for all architectures!
		bc.ImageConfiguration.Archs = types.AllArchs
	}
	return bc, nil
}

type imagesbom struct {
	imageHash     v1.Hash
	predicateType string
	predicate     []byte
}

func doBuild(ctx context.Context, data BuildResourceModel, ropts []remote.Option) (v1.Hash, coci.SignedEntity, map[string]imagesbom, error) {
	tempDir, err := os.MkdirTemp("", "apko-*")
	if err != nil {
		return v1.Hash{}, nil, nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	workDir := filepath.Join(tempDir, "builds")
	defer os.RemoveAll(tempDir)

	var ic types.ImageConfiguration
	if diags := assignValue(data.Config, &ic); diags.HasError() {
		return v1.Hash{}, nil, nil, fmt.Errorf("assigning value: %v", diags.Errors())
	}

	tflog.Trace(ctx, fmt.Sprintf("Got image configuration: %#v", ic))

	// Parse things once to determine the architectures to build from
	// the config.
	obc, err := fromImageData(ic, data.popts, workDir)
	if err != nil {
		return v1.Hash{}, nil, nil, err
	}
	obc.Options.SBOMPath = tempDir

	var errg errgroup.Group
	imgs := make(map[types.Architecture]coci.SignedImage, len(obc.ImageConfiguration.Archs))

	sboms := make(map[string]imagesbom, len(obc.ImageConfiguration.Archs)+1)

	for _, arch := range obc.ImageConfiguration.Archs {
		arch := arch

		bc, err := fromImageData(ic, data.popts, filepath.Join(workDir, arch.ToAPK()))
		if err != nil {
			return v1.Hash{}, nil, nil, err
		}

		errg.Go(func() error {
			ropts := append(ropts, remote.WithContext(ctx))

			bc.Options.Arch = arch

			if err := bc.Refresh(); err != nil {
				return fmt.Errorf("failed to update build context for %q: %w", arch, err)
			}
			bc.Options.SBOMPath = tempDir
			// Don't build the SBOM when we make the layer, since we want to
			// set the creation timestamp based on the build-date-epoch.
			bc.Options.WantSBOM = false

			_, layer, err := bc.BuildLayer()
			if err != nil {
				return fmt.Errorf("failed to build layer image for %q: %w", arch, err)
			}
			// TODO(kaniini): clean up everything correctly for multitag scenario
			// defer os.Remove(layerTarGZ)

			if bc.Options.SourceDateEpoch, err = bc.GetBuildDateEpoch(); err != nil {
				return fmt.Errorf("failed to determine build date epoch: %w", err)
			}
			bc.Options.SourceDateEpoch = bc.Options.SourceDateEpoch.UTC()
			// Adjust the index's builder to track the most recent BDE.
			if bc.Options.SourceDateEpoch.After(obc.Options.SourceDateEpoch) {
				obc.Options.SourceDateEpoch = bc.Options.SourceDateEpoch
			}

			// Explicitly generate the SBOM after the BDE calculation
			if err := bc.GenerateSBOM(); err != nil {
				return fmt.Errorf("failed to determine build date epoch: %w", err)
			}

			_, img, err := oci.PublishImageFromLayer(
				ctx, layer, bc.ImageConfiguration, bc.Options.SourceDateEpoch, arch, bc.Logger(),
				false /* local */, true /* shouldPushTags */, []string{} /* tags */, ropts...,
			)
			if err != nil {
				return fmt.Errorf("failed to build OCI image for %q: %w", arch, err)
			}
			h, err := img.Digest()
			if err != nil {
				return fmt.Errorf("unable to compute digest for %q: %w", arch, err)
			}
			content, err := os.ReadFile(filepath.Join(tempDir, fmt.Sprintf("sbom-%s.spdx.json", arch.ToAPK())))
			if err != nil {
				return fmt.Errorf("unable to read SBOM %q: %w", arch, err)
			}

			imgs[arch] = img
			sboms[arch.String()] = imagesbom{
				imageHash:     h,
				predicateType: "https://spdx.dev/Document",
				predicate:     content,
			}
			return nil
		})
	}

	if err := errg.Wait(); err != nil {
		return v1.Hash{}, nil, nil, err
	}
	// If we built a final image, then return that instead of wrapping it in an
	// image index.
	if len(imgs) == 1 {
		for _, img := range imgs {
			h, err := img.Digest()
			if err != nil {
				return v1.Hash{}, nil, nil, err
			}
			return h, img, sboms, nil
		}
	}

	idx := signed.ImageIndex(mutate.IndexMediaType(empty.Index, ggcrtypes.OCIImageIndex))
	archs := make([]types.Architecture, 0, len(imgs))
	for arch := range imgs {
		archs = append(archs, arch)
	}
	sort.Slice(archs, func(i, j int) bool {
		return archs[i].String() < archs[j].String()
	})
	for _, arch := range archs {
		img := imgs[arch]
		mt, err := img.MediaType()
		if err != nil {
			return v1.Hash{}, nil, nil, fmt.Errorf("failed to get mediatype: %w", err)
		}

		h, err := img.Digest()
		if err != nil {
			return v1.Hash{}, nil, nil, fmt.Errorf("failed to compute digest: %w", err)
		}

		size, err := img.Size()
		if err != nil {
			return v1.Hash{}, nil, nil, fmt.Errorf("failed to compute size: %w", err)
		}

		idx = ocimutate.AppendManifests(idx, ocimutate.IndexAddendum{
			Add: img,
			Descriptor: v1.Descriptor{
				MediaType: mt,
				Digest:    h,
				Size:      size,
				Platform:  arch.ToOCIPlatform(),
			},
		})
	}

	h, err := idx.Digest()
	if err != nil {
		return v1.Hash{}, nil, nil, err
	}

	// Only the v1.Hash is needed, the rest is discarded by apko...
	finalDigest, _ := name.NewDigest("ubuntu@" + h.String())

	isboms, err := obc.GenerateIndexSBOM(finalDigest, imgs)
	if err != nil {
		return v1.Hash{}, nil, nil, fmt.Errorf("generating index SBOM: %w", err)
	}
	content, err := os.ReadFile(isboms[0].Path)
	if err != nil {
		return v1.Hash{}, nil, nil, fmt.Errorf("unable to read index SBOM: %w", err)
	}
	sboms["index"] = imagesbom{
		imageHash:     h,
		predicateType: "https://spdx.dev/Document",
		predicate:     content,
	}
	return h, idx, sboms, nil
}
