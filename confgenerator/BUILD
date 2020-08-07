go_library(
    name = "confgenerator",
    srcs = ["confgenerator.go"],
    deps = [
        "//cloud/monitoring/agents/unified_agents/fluentbit/conf",
        "//third_party/golang/yaml",
    ],
)

go_test(
    name = "confgenerator_test",
    size = "small",
    srcs = ["confgenerator_test.go"],
    data = glob(["testdata/**"]),
    library = ":confgenerator",
    deps = [
        "//base/go:runfiles",
        "//third_party/golang/cmp",
    ],
)
