load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "replica",
    srcs = ["replica.go"],
    importpath = "github.com/buildbuddy-io/buildbuddy/enterprise/server/raft/replica",
    visibility = ["//visibility:public"],
    deps = [
        "//enterprise/server/raft/constants",
        "//enterprise/server/raft/filestore",
        "//enterprise/server/raft/keys",
        "//enterprise/server/raft/rbuilder",
        "//enterprise/server/raft/sender",
        "//proto:raft_go_proto",
        "//server/util/log",
        "//server/util/rangemap",
        "//server/util/status",
        "@com_github_cockroachdb_pebble//:pebble",
        "@com_github_lni_dragonboat_v3//statemachine",
        "@go_googleapis//google/rpc:status_go_proto",
        "@org_golang_google_grpc//status",
        "@org_golang_google_protobuf//encoding/prototext",
        "@org_golang_google_protobuf//proto",
    ],
)

go_test(
    name = "replica_test",
    srcs = ["replica_test.go"],
    deps = [
        ":replica",
        "//enterprise/server/raft/constants",
        "//enterprise/server/raft/keys",
        "//enterprise/server/raft/rbuilder",
        "//enterprise/server/raft/sender",
        "//proto:raft_go_proto",
        "//server/testutil/testdigest",
        "//server/testutil/testfs",
        "//server/util/status",
        "@com_github_cockroachdb_pebble//:pebble",
        "@com_github_lni_dragonboat_v3//statemachine",
        "@com_github_stretchr_testify//require",
        "@org_golang_google_protobuf//proto",
    ],
)
