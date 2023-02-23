package provider

import (
	"context"
	"os"

	"chainguard.dev/apko/pkg/build"
	"chainguard.dev/apko/pkg/build/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/sigstore/cosign/v2/pkg/oci"
	"gopkg.in/yaml.v3"
)

func resourceApkoBuild() *schema.Resource {
	return &schema.Resource{
		Description: "This performs an apko build from the provided config file",

		CreateContext: resourceApkoBuildCreate,
		ReadContext:   resourceApkoBuildRead,
		DeleteContext: resourceApkoBuildDelete,

		Schema: map[string]*schema.Schema{
			"repo": {
				Description: "The name of the container repository to which we should publish the image.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				ValidateDiagFunc: func(data interface{}, _ cty.Path) diag.Diagnostics {
					raw, ok := data.(string)
					if !ok {
						return diag.Errorf("%v is a %T, wanted a string", data, data)
					}
					_, err := name.NewRepository(raw)
					return diag.FromErr(err)
				},
			},
			"config": {
				Description: "The apko configuration file.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				// TODO: Add validation of the apko config.
			},
			"image_ref": {
				Description: "The resulting fully-qualified digest (e.g. {repo}@sha256:deadbeef).",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

func fromImageData(d *schema.ResourceData, wd string) (*build.Context, error) {
	opts := []build.Option{}

	var ic types.ImageConfiguration
	if err := yaml.Unmarshal([]byte(d.Get("config").(string)), &ic); err != nil {
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

func resourceApkoBuildCreate(ctx context.Context, d *schema.ResourceData, _ interface{}) diag.Diagnostics {
	wd, err := os.MkdirTemp("", "apko-*")
	if err != nil {
		return diag.Errorf("failed to create working directory: %v", err)
	}
	defer os.RemoveAll(wd)

	repo, err := name.NewRepository(d.Get("repo").(string))
	if err != nil {
		return diag.FromErr(err)
	}
	h, se, err := doBuild(ctx, d, wd)
	if err != nil {
		return diag.Errorf("doBuild: %v", err)
	}
	ref := repo.Digest(h.String())

	kc := authn.NewMultiKeychain(
		authn.DefaultKeychain,
		// TODO: build in cred helpers.
	)

	switch i := se.(type) {
	case oci.SignedImage:
		if err := remote.Write(ref, i, remote.WithAuthFromKeychain(kc)); err != nil {
			return diag.FromErr(err)
		}
	case oci.SignedImageIndex:
		if err := remote.WriteIndex(ref, i, remote.WithAuthFromKeychain(kc)); err != nil {
			return diag.FromErr(err)
		}
	default:
		return diag.Errorf("wanted an image or index, but got %T", se)
	}

	d.Set("image_ref", ref.String())
	d.SetId(ref.String())
	return nil
}

func resourceApkoBuildRead(ctx context.Context, d *schema.ResourceData, _ interface{}) diag.Diagnostics {
	wd, err := os.MkdirTemp("", "apko-*")
	if err != nil {
		return diag.Errorf("failed to create working directory: %v", err)
	}
	defer os.RemoveAll(wd)

	repo, err := name.NewRepository(d.Get("repo").(string))
	if err != nil {
		return diag.FromErr(err)
	}
	h, _, err := doBuild(ctx, d, wd)
	if err != nil {
		return diag.Errorf("doBuild: %v", err)
	}

	ref := repo.Digest(h.String()).String()

	d.Set("image_ref", ref)
	d.SetId(ref)
	return nil
}

func resourceApkoBuildDelete(ctx context.Context, d *schema.ResourceData, _ interface{}) diag.Diagnostics {
	// TODO: If we ever want to delete the image from the registry, we can do it here.
	return nil
}
