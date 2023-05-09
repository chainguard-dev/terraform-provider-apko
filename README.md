# Terraform Provider for [`apko`](https://github.com/chainguard-dev/apko)

🚨 **This is a work in progress.** 🚨

https://registry.terraform.io/providers/chainguard-dev/apko

## Usage

This provides an `apko_build` resource that will build the provided `apko` configuration, push an image to the configured container repository, and make the image's reference available to other Terraform resources.

```hcl

# Define provider-wide defaults to reduce boilerplate or augment built images
# with additional packages.
provider "apko" {
  # Default to building for these architectures.
  default_archs = ["x86_64", "aarch64"]

  # Include these repositories by default
  extra_repositories = ["https://packages.wolfi.dev/os"]
  extra_keyring      = ["https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"]

  # All of the images should show up as Wolfi!
  extra_packages     = ["wolfi-baselayout"]
}

data "apko_config" "this" {
  # Pass in the apko configuration here.  If you'd like to define this in a file
  # so it can be used with apko as well, you can make this something like this
  # instead:  config = file("${path.module}/apko.yaml")
  config_contents = jsonencode({
    contents = {
      packages = [
        "ca-certificates-bundle",
        "tzdata"
      ]
    },
    accounts = {
      groups = [{
        groupname = "nonroot",
        gid = 65532
      }],
      users = [{
        username = "nonroot",
        uid = 65532,
        gid = 65532
      }],
      run-as = 65532
    },
  })
}

resource "apko_build" "this" {
  # Where to publish the resulting image, e.g. docker.io/user/repo
  repo   = "..."
  config = data.apko_config.this.config
}
```

The image will be rebuilt every time it's _referenced_, and will only report as having changed if the image that was built was different since the last time the image resource was read.

This means that `terraform plan` will rebuild all referenced images, but only show diffs if rebuilds resulted in new images since last time the plan was made.
