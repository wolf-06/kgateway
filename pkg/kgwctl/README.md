# kgwctl
kgwctl is a command-line tool for managing and understanding kgateway resources in your Kubernetes cluster. It allows you to view, describe, check and analyze Gateway API and other Kubernetes resources. It also provides advanced features like visualizing relationships with DOT graphs and warning about potential issues.

Note: This tool is a fork of [gwctl](https://github.com/kubernetes-sigs/gwctl) with the goal to be a more opinionated tool to better serve the needs of kgateway users.

## Installation
Prerequisit to run the script:
- [jq](https://jqlang.org/)
Execute the following command:
```sh
curl -sL -H "Authorization: token ${GITHUB_TOKEN}" https://raw.githubusercontent.com/solo-io/kgwctl-temp/refs/heads/main/scripts/install.sh | sh -
```

To download a specific version you can specify it like so:
```sh
curl -sL -H "Authorization: token ${GITHUB_TOKEN}" https://raw.githubusercontent.com/solo-io/kgwctl-temp/refs/heads/main/scripts/install.sh | VERSION="v0.0.1" sh -
```

## Build and Release
To use the cli locally you can either use it directly with
```shell
go run main.go ...
```
or build the appropriate binary with one of
```shell
make build-darwin-amd64
make build-darwin-arm64
make build-linux-amd64
make build-linux-arm64
```

## Usage


### Check installation
The `check` command will check the state of the kgateway controller and surfaces any issues found in gateways that use GatewayClasses it is responsible for.
```sh
kgwctl check

# OUTPUT (colors not shown):
Controller: kgateway.dev/kgateway
- deployment: kgateway | healthy

GatewayClasses:
- name: kgateway
  Gateways:
  - example/gw2 - 2 issues found
    - HTTPRoute.gateway.networking.k8s.io/example/example-route-2: HTTPRoute(.gateway.networking.k8s.io) "example/example-route-2" references a non-existent Backend(.gateway.kgateway.dev) "example/missing-backend-example"
    - HTTPRoute.gateway.networking.k8s.io/example/example-route: HTTPRoute(.gateway.networking.k8s.io) "example/example-route" references a non-existent Service "example/example-svc-2"
  - kgateway-system/gw - 0 issues found

- name: kgateway-waypoint
  Gateways:
  - None found.
```