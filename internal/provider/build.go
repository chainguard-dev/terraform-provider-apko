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
	ggcrtypes "github.com/google/go-containerregistry/pkg/v1/types"
	coci "github.com/sigstore/cosign/v2/pkg/oci"
	ocimutate "github.com/sigstore/cosign/v2/pkg/oci/mutate"
	"github.com/sigstore/cosign/v2/pkg/oci/signed"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"
)

func fromImageData(data BuildResourceModel, wd string) (*build.Context, error) {
	var ic types.ImageConfiguration
	if err := yaml.Unmarshal([]byte(data.Config.ValueString()), &ic); err != nil {
		return nil, err
	}

	ic.Contents.Packages = append(ic.Contents.Packages, data.popts.packages...)

	bc, err := build.New(wd,
		build.WithImageConfiguration(ic),
		build.WithSBOMFormats([]string{"spdx"}),
		build.WithExtraKeys(data.popts.keyring),
		build.WithExtraRepos(data.popts.repositories),
	)

	if err != nil {
		return nil, err
	}

	if len(data.popts.archs) != 0 {
		bc.ImageConfiguration.Archs = types.ParseArchitectures(data.popts.archs)
	} else if len(bc.ImageConfiguration.Archs) == 0 {
		bc.ImageConfiguration.Archs = types.AllArchs
	}
	return bc, nil
}

type imagesbom struct {
	imageHash     v1.Hash
	predicateType string
	predicate     []byte
}

func doBuild(ctx context.Context, data BuildResourceModel) (v1.Hash, coci.SignedEntity, map[string]imagesbom, error) {
	tempDir, err := os.MkdirTemp("", "apko-*")
	if err != nil {
		return v1.Hash{}, nil, nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	workDir := filepath.Join(tempDir, "builds")
	defer os.RemoveAll(tempDir)

	// Parse things once to determine the architectures to build from
	// the config.
	obc, err := fromImageData(data, workDir)
	if err != nil {
		return v1.Hash{}, nil, nil, err
	}
	obc.Options.SBOMPath = tempDir

	var errg errgroup.Group
	imgs := make(map[types.Architecture]coci.SignedImage, len(obc.ImageConfiguration.Archs))

	sboms := make(map[string]imagesbom, len(obc.ImageConfiguration.Archs)+1)

	for _, arch := range obc.ImageConfiguration.Archs {
		arch := arch

		bc, err := fromImageData(data, filepath.Join(workDir, arch.ToAPK()))
		if err != nil {
			return v1.Hash{}, nil, nil, err
		}

		errg.Go(func() error {
			bc.Options.Arch = arch

			if err := bc.Refresh(); err != nil {
				return fmt.Errorf("failed to update build context for %q: %w", arch, err)
			}
			bc.Options.SBOMPath = tempDir

			layerTarGZ, err := bc.BuildLayer()
			if err != nil {
				return fmt.Errorf("failed to build layer image for %q: %w", arch, err)
			}
			// TODO(kaniini): clean up everything correctly for multitag scenario
			// defer os.Remove(layerTarGZ)

			_, img, err := oci.PublishImageFromLayer(
				ctx, layerTarGZ, bc.ImageConfiguration, bc.Options.SourceDateEpoch, arch, bc.Logger(),
				tempDir, bc.Options.SBOMFormats, false /* local */, true, /* shouldPushTags */
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
			sboms[arch.ToAPK()] = imagesbom{
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

	if err := obc.GenerateIndexSBOM(finalDigest, imgs); err != nil {
		return v1.Hash{}, nil, nil, fmt.Errorf("generating index SBOM: %w", err)
	}
	content, err := os.ReadFile(filepath.Join(tempDir, "sbom-index.spdx.json"))
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
