package provider

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceApkoImage() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "Sample resource in the Terraform provider ApkoImage.",

		CreateContext: resourceApkoImageCreate,
		ReadContext:   resourceApkoImageRead,
		DeleteContext: resourceApkoImageDelete,

		Schema: map[string]*schema.Schema{
			"sample_attribute": {
				// This description is used by the documentation generator and the language server.
				Description: "Sample attribute.",
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
			"image_ref": {
				Description: "built image reference by digest",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

type buildOptions struct {
	// TODO
}

func fromData(d *schema.ResourceData, repo string) buildOptions {
	return buildOptions{
		// TODO
	}
}

func doBuild(ctx context.Context, opts buildOptions) (string, error) {
	return "TODO", nil
}

func resourceApkoImageCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	ref, err := doBuild(ctx, fromData(d, meta.(string)))
	if err != nil {
		return diag.Errorf("doBuild: %v", err)
	}

	d.Set("image_ref", ref)
	d.SetId(ref)
	return nil
}

func resourceApkoImageRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	ref, err := doBuild(ctx, fromData(d, meta.(string)))
	if err != nil {
		return diag.Errorf("doBuild: %v", err)
	}

	d.Set("image_ref", ref)
	if ref != d.Id() {
		d.SetId("")
	} else {
		log.Println("image not changed")
	}
	return nil
}

func resourceApkoImageDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// TODO: If we ever want to delete the image from the registry, we can do it here.
	return nil
}
