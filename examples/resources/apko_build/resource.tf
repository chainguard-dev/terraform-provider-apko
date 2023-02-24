resource "apko_build" "example" {
  # Where to publish the resulting image, e.g. docker.io/user/repo
  repo = "..."

  # Pass in the apko configuration here.  If you'd like to define this in a file
  # so it can be used with apko as well, you can make this something like this
  # instead:  config = file("${path.module}/apko.yaml")
  config = jsonencode({
    contents = {
      repositories = ["https://packages.wolfi.dev/os"]
      keyring      = ["https://packages.wolfi.dev/os/wolfi-signing.rsa.pub"]
      packages = [
        "wolfi-baselayout",
        "ca-certificates-bundle",
        "tzdata"
      ]
    },
    accounts = {
      groups = [{
        groupname = "nonroot",
        gid       = 65532
      }],
      users = [{
        username = "nonroot",
        uid      = 65532,
        gid      = 65532
      }],
      run-as = 65532
    },
    archs = [
      "x86_64",
      "aarch64"
    ]
  })
}
