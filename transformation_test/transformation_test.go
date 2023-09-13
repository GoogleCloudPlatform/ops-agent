package transformation_test

import (
	"bytes"
	"embed"
	"fmt"
	"path"
	"testing"

	"github.com/goccy/go-yaml"
)

//go:embed testdata
var testdataDir embed.FS

type test struct {
	testName string
}

type TransformationTest struct {
	log []string `yaml:",inline"`
}

type TransformationTestInput struct {
	input    string `yaml:"input"`
	expected string `yaml:"expected"`
}

func TestTransformationTests(t *testing.T) {
	allTests, err := testdataDir.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}

	for _, dir := range allTests {
		if !dir.IsDir() {
			t.Fatal("testdata folder must only contain folders")
		}

		var transformationTestData []byte
		transformationTestData, err = readFileFromTestDir(path.Join(dir.Name(), "transformation_test.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		transformationTestData = bytes.ReplaceAll(transformationTestData, []byte("\r\n"), []byte("\n"))

		testInput := &TransformationTestInput{}
		err = yaml.UnmarshalWithOptions(transformationTestData, testInput, yaml.DisallowUnknownField())
		if err != nil {
			t.Fatal(err)
		}

		fmt.Println(testInput)
	}
}

func readFileFromTestDir(filePath string) ([]byte, error) {
	return testdataDir.ReadFile(path.Join("testdataDir", filePath))
}
