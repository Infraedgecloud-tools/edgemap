# edgemap

A Terraform/OpenTofu architecture diagram CLI. Implementation is in progress;
there is no published installable release yet.

## Development

Go 1.27.3:

```sh
go run ./cmd/edgemap version
go test ./...
```

Changes go through pull requests with Conventional Commits and passing CI.
Do not commit Terraform plans, state, secrets or customer configuration.

Licensed under Apache-2.0. Third-party notices will accompany release archives.

Built and maintained by [InfraEdge](https://infraedge.org).
