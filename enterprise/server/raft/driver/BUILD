load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "driver",
    srcs = ["driver.go"],
    importpath = "github.com/buildbuddy-io/buildbuddy/enterprise/server/raft/driver",
    visibility = ["//visibility:public"],
    deps = [
        "//enterprise/server/raft/constants",
        "//proto:raft_go_proto",
        "//proto:raft_service_go_proto",
        "//server/gossip",
        "//server/util/log",
        "//server/util/status",
        "@com_github_hashicorp_serf//serf",
        "@org_golang_google_protobuf//proto",
    ],
)
