load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

# gazelle:resolve go github.com/bazelbuild/bazelisk/core @com_github_bazelbuild_bazelisk//core:go_default_library
# gazelle:resolve go github.com/bazelbuild/bazelisk/repositories @com_github_bazelbuild_bazelisk//repositories:go_default_library

go_library(
    name = "bb_lib",
    srcs = ["bb.go"],
    importpath = "github.com/buildbuddy-io/buildbuddy/cli/cmd/bb",
    visibility = ["//visibility:private"],
    deps = [
        "//cli/autoconfig",
        "//cli/commandline",
        "//cli/logging",
        "//cli/parser",
        "//cli/remotebazel",
        "//cli/sidecar",
        "//proto:sidecar_go_proto",
        "//server/util/grpc_client",
        "//server/version",
        "@com_github_bazelbuild_bazelisk//core:go_default_library",
        "@com_github_bazelbuild_bazelisk//repositories:go_default_library",
    ],
)

go_binary(
    name = "bb",
    embed = [":bb_lib"],
    visibility = ["//visibility:public"],
)

go_binary(
    name = "bb-darwin-amd64",
    out = "bb-darwin-amd64",
    embed = [":bb_lib"],
    gc_linkopts = [
        "-s",
        "-w",
    ],
    goarch = "amd64",
    goos = "darwin",
    pure = "on",
    tags = ["manual"],
    visibility = ["//visibility:public"],
)

go_binary(
    name = "bb-darwin-arm64",
    out = "bb-darwin-arm64",
    embed = [":bb_lib"],
    gc_linkopts = [
        "-s",
        "-w",
    ],
    goarch = "arm64",
    goos = "darwin",
    pure = "on",
    tags = ["manual"],
    visibility = ["//visibility:public"],
)

go_binary(
    name = "bb-linux-amd64",
    out = "bb-linux-amd64",
    embed = [":bb_lib"],
    gc_linkopts = [
        "-s",
        "-w",
    ],
    goarch = "amd64",
    goos = "linux",
    pure = "on",
    tags = ["manual"],
    visibility = ["//visibility:public"],
)

go_binary(
    name = "bb-linux-arm64",
    out = "bb-linux-arm64",
    embed = [":bb_lib"],
    gc_linkopts = [
        "-s",
        "-w",
    ],
    goarch = "arm64",
    goos = "linux",
    pure = "on",
    tags = ["manual"],
    visibility = ["//visibility:public"],
)

go_binary(
    name = "bb-windows-amd64",
    out = "bb-windows-amd64.exe",
    embed = [":bb_lib"],
    goarch = "amd64",
    goos = "windows",
    pure = "on",
    tags = ["manual"],
    visibility = ["//visibility:public"],
)
