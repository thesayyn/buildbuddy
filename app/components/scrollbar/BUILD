load("//rules/typescript:index.bzl", "ts_library")

package(default_visibility = ["//visibility:public"])

ts_library(
    name = "scrollbar",
    srcs = glob([
        "*.ts",
        "*.tsx",
    ]),
    deps = [
        "//:node_modules/@types/react",
        "//:node_modules/@types/uuid",
        "//:node_modules/react",
        "//:node_modules/rxjs",
        "//:node_modules/tslib",
        "//:node_modules/uuid",
    ],
)
