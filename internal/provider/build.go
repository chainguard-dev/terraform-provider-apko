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
	opts := []build.Option{}

	var ic types.ImageConfiguration
	if err := yaml.Unmarshal([]byte(data.Config.ValueString()), &ic); err != nil {
		return nil, err
	}
	opts = append(opts,
		build.WithImageConfiguration(ic),
		// TODO(mattmoor): SBOMs would be nice
	)

	bc, err := build.New(wd, opts...)
	if err != nil {
		return nil, err
	}

	bc.Options.WantSBOM = len(bc.Options.SBOMFormats) > 0
	if len(bc.ImageConfiguration.Archs) == 0 {
		bc.ImageConfiguration.Archs = types.AllArchs
	}
	return bc, nil
}

func doBuild(ctx context.Context, data BuildResourceModel) (v1.Hash, coci.SignedEntity, error) {
	wd, err := os.MkdirTemp("", "apko-*")
	if err != nil {
		return v1.Hash{}, nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(wd)

	// Parse things once to determine the architectures to build from
	// the config.
	obc, err := fromImageData(data, wd)
	if err != nil {
		return v1.Hash{}, nil, err
	}

	var errg errgroup.Group
	imgs := make(map[types.Architecture]coci.SignedImage, len(obc.ImageConfiguration.Archs))

	for _, arch := range obc.ImageConfiguration.Archs {
		arch := arch

		bc, err := fromImageData(data, filepath.Join(wd, arch.ToAPK()))
		if err != nil {
			return v1.Hash{}, nil, err
		}
		// This is a hack to skip the SBOM generation during
		// image build. Will be removed when global options are a thing.
		bc.Options.SBOMFormats = []string{}
		bc.Options.WantSBOM = false

		errg.Go(func() error {
			bc.Options.Arch = arch

			if err := bc.Refresh(); err != nil {
				return fmt.Errorf("failed to update build context for %q: %w", arch, err)
			}

			layerTarGZ, err := bc.BuildLayer()
			if err != nil {
				return fmt.Errorf("failed to build layer image for %q: %w", arch, err)
			}
			// TODO(kaniini): clean up everything correctly for multitag scenario
			// defer os.Remove(layerTarGZ)

			_, img, err := oci.PublishImageFromLayer(
				layerTarGZ, bc.ImageConfiguration, bc.Options.SourceDateEpoch, arch, bc.Logger(),
				bc.Options.SBOMPath, bc.Options.SBOMFormats, false /* local */, true, /* shouldPushTags */
			)
			if err != nil {
				return fmt.Errorf("failed to build OCI image for %q: %w", arch, err)
			}

			imgs[arch] = img
			return nil
		})
	}

	if err := errg.Wait(); err != nil {
		return v1.Hash{}, nil, err
	}
	// If we built a final image, then return that instead of wrapping it in an
	// image index.
	if len(imgs) == 1 {
		for _, img := range imgs {
			h, err := img.Digest()
			if err != nil {
				return v1.Hash{}, nil, err
			}
			return h, img, nil
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
			return v1.Hash{}, nil, fmt.Errorf("failed to get mediatype: %w", err)
		}

		h, err := img.Digest()
		if err != nil {
			return v1.Hash{}, nil, fmt.Errorf("failed to compute digest: %w", err)
		}

		size, err := img.Size()
		if err != nil {
			return v1.Hash{}, nil, fmt.Errorf("failed to compute size: %w", err)
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
		return v1.Hash{}, nil, err
	}

	return h, idx, nil

}
