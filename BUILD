load("@aspect_bazel_lib//lib:copy_to_bin.bzl", "copy_to_bin")
load("@bazel_gazelle//:def.bzl", "gazelle")
load("@io_bazel_rules_go//go:def.bzl", "go_library", "nogo")
load("@aspect_rules_ts//ts:defs.bzl", "ts_config")
load("@npm//:defs.bzl", "npm_link_all_packages")

package(default_visibility = ["//visibility:public"])

npm_link_all_packages(name = "node_modules")

nogo(
    name = "vet",
    config = "nogo_config.json",
    vet = True,
    visibility = ["//visibility:public"],
    deps = [
        "@org_golang_x_tools//go/analysis/passes/asmdecl:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/assign:go_default_library",
        # "@org_golang_x_tools//go/analysis/passes/atomic:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/atomicalign:go_default_library",
        # "@org_golang_x_tools//go/analysis/passes/bools:go_default_library",
        # "@org_golang_x_tools//go/analysis/passes/buildtag:go_default_library",
        # "@org_golang_x_tools//go/analysis/passes/cgocall:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/composite:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/copylock:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/deepequalerrors:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/errorsas:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/fieldalignment:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/framepointer:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/httpresponse:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/ifaceassert:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/loopclosure:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/lostcancel:go_default_library",
        # "@org_golang_x_tools//go/analysis/passes/nilfunc:go_default_library",
        # "@org_golang_x_tools//go/analysis/passes/nilness:go_default_library", # template methods currently cause this analyzer to panic
        # "@org_golang_x_tools//go/analysis/passes/printf:go_default_library",
        # "@org_golang_x_tools//go/analysis/passes/shadow:go_default_library", # Everyone shadows `err`
        "@org_golang_x_tools//go/analysis/passes/shift:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/sortslice:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/stdmethods:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/stringintconv:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/structtag:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/tests:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/testinggoroutine:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/unmarshal:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/unreachable:go_default_library",
        # "@org_golang_x_tools//go/analysis/passes/unsafeptr:go_default_library",
        "@org_golang_x_tools//go/analysis/passes/unusedresult:go_default_library",
        "@com_github_nishanths_exhaustive//:exhaustive",
    ],
)

# Ignore the node_modules dir
# gazelle:exclude node_modules
# Ignore generated proto files
# gazelle:exclude **/*.pb.go
# gazelle:exclude bundle.go
# gazelle:exclude enterprise/bundle.go
# Prefer generated BUILD files to be called BUILD over BUILD.bazel
# gazelle:build_file_name BUILD,BUILD.bazel
# gazelle:prefix github.com/buildbuddy-io/buildbuddy
# gazelle:proto disable
gazelle(name = "gazelle")

exports_files([
    "package.json",
    "yarn.lock",
    "VERSION",
])

ts_config(
    name = "tsconfig",
    src = ":tsconfig.json",
)

filegroup(
    name = "config_files",
    srcs = select({
        ":release_build": ["config/buildbuddy.release.yaml"],
        "//conditions:default": glob(["config/**"]),
    }),
)

config_setting(
    name = "release_build",
    values = {"define": "release=true"},
)

package_group(
    name = "os",
    packages = [
        "//app/...",
        "//config/...",
        "//deployment/...",
        "//docs/...",
        "//node_modules/...",
        "//proto/...",
        "//rules/...",
        "//server/...",
        "//static/...",
        "//templates/...",
        "//tools/...",
    ],
)

package_group(
    name = "enterprise",
    packages = [
        "//enterprise/...",
    ],
)

config_setting(
    name = "fastbuild",
    values = {"compilation_mode": "fastbuild"},
)

# N.B. this is ignored by gazelle so must be updated by hand.
# It must live at the repo root to be able to bundle other files using
# "go:embed".
go_library(
    name = "bundle",
    srcs = ["bundle.go"],
    embedsrcs = select({
        ":fastbuild": [
            "//:config_files",
            "//static",
        ],
        "//conditions:default": [
            "//:config_files",
            "//app:app_bundle",
            "//app:style.css",
            "//app:sha",
            "//static",
        ],
    }),
    importpath = "github.com/buildbuddy-io/buildbuddy",
    deps = [
        "//server/util/fileresolver",
    ],
)

platform(
    name = "firecracker",
    constraint_values = [
        "@bazel_tools//platforms:x86_64",
        "@bazel_tools//platforms:linux",
    ],
    exec_properties = {
        "workload-isolation-type": "firecracker",
    },
)

platform(
    name = "firecracker_vfs",
    constraint_values = [
        "@bazel_tools//platforms:x86_64",
        "@bazel_tools//platforms:linux",
    ],
    exec_properties = {
        "workload-isolation-type": "firecracker",
        "enable-vfs": "true",
    },
)

platform(
    name = "vfs",
    constraint_values = [
        "@bazel_tools//platforms:x86_64",
        "@bazel_tools//platforms:linux",
    ],
    exec_properties = {
        "enable-vfs": "true",
    },
)

# TODO(bduffany): The sh_toolchain config here is a workaround for
# https://github.com/aspect-build/rules_swc/issues/20
# We should probably either move these to the buildbuddy-toolchain repo
# or add a symlink from /usr/bin/bash -> /bin/bash to remove the need for these.
load("@bazel_tools//tools/sh:sh_toolchain.bzl", "sh_toolchain")

sh_toolchain(
    name = "bash_rbe_ubuntu1604",
    path = "/bin/bash",
)

toolchain(
    name = "sh_toolchain",
    toolchain = ":bash_rbe_ubuntu1604",
    toolchain_type = "@bazel_tools//tools/sh:toolchain_type",
)

copy_to_bin(
    name = "swcrc",
    srcs = [".swcrc"],
)
