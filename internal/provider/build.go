package provider

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"chainguard.dev/apko/pkg/build"
	"chainguard.dev/apko/pkg/build/oci"
	"chainguard.dev/apko/pkg/build/types"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	ggcrtypes "github.com/google/go-containerregistry/pkg/v1/types"
	coci "github.com/sigstore/cosign/pkg/oci"
	ocimutate "github.com/sigstore/cosign/pkg/oci/mutate"
	"github.com/sigstore/cosign/pkg/oci/signed"
	"golang.org/x/sync/errgroup"
)

func doBuild(ctx context.Context, bc *build.Context) (v1.Hash, coci.SignedEntity, error) {
	var errg errgroup.Group
	workDir := bc.Options.WorkDir
	imgs := map[types.Architecture]coci.SignedImage{}

	// This is a hack to skip the SBOM generation during
	// image build. Will be removed when global options are a thing.
	bc.Options.SBOMFormats = []string{}
	bc.Options.WantSBOM = false

	for _, arch := range bc.ImageConfiguration.Archs {
		arch := arch
		bc := *bc

		errg.Go(func() error {
			bc.Options.Arch = arch
			bc.Options.WorkDir = filepath.Join(workDir, arch.ToAPK())

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
