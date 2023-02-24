# Terraform Provider for [`apko`](https://github.com/chainguard-dev/apko)

🚨 **This is a work in progress.** 🚨

https://registry.terraform.io/providers/chainguard-dev/apko

## Usage

This provides an `apko_build` resource that will build the provided `apko` configuration, push an image to the configured container repository, and make the image's reference available to other Terraform resources.

```hcl
provider "apko" {}

resource "apko_build" "example" {
  # Where to publish the resulting image, e.g. docker.io/user/repo
  repo   = "..."

  # Pass in the apko configuration here.  If you'd like to define this in a file
  # so it can be used with apko as well, you can make this something like this
  # instead:  config = file("${path.module}/apko.yaml")
  config = jsonencode({
    contents = {
      repositories = ["https://packages.wolfi.dev/os"]
      keyring = ["https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"]
      packages = [
        "wolfi-baselayout",
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
    archs = [
      "x86_64",
      "aarch64"
    ]
  })
}
```

The image will be rebuilt every time it's _referenced_, and will only report as having changed if the image that was built was different since the last time the image resource was read.

This means that `terraform plan` will rebuild all referenced images, but only show diffs if rebuilds resulted in new images since last time the plan was made.
