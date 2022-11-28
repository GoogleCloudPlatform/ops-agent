package set_test

import (
	"sort"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/internal/set"
	"gotest.tools/v3/assert"
)

func TestSliceToSet(t *testing.T) {
	testSlice := []int{
		1,
		2,
	}
	testSet := set.FromSlice(testSlice)
	assert.Equal(t, len(testSet), len(testSlice))
	for _, v := range testSlice {
		assert.Assert(
			t,
			testSet.Contains(v),
			"Set was missing key from original slice %s",
			v,
		)
	}
}

func TestMapToSet(t *testing.T) {
	testMap := map[string]string{
		"key1": "",
		"key2": "",
		"key3": "",
	}
	testSet := set.FromMapKeys(testMap)
	assert.Equal(t, len(testSet), len(testMap))
	for k := range testMap {
		assert.Assert(
			t,
			testSet.Contains(k),
			"Set was missing key from original map %s",
			k,
		)
	}
}

func TestAdd(t *testing.T) {
	testSet := set.Set[string]{}
	newKey := "new key!"
	testSet.Add(newKey)
	assert.Equal(t, len(testSet), 1)
	assert.Assert(t, testSet.Contains(newKey))
}

func TestRemove(t *testing.T) {
	key := "key"
	testSet := set.Set[string]{
		key: struct{}{},
	}
	testSet.Remove(key)
	assert.Assert(t, !testSet.Contains(key))
	assert.Equal(t, len(testSet), 0)
}

func TestContains(t *testing.T) {
	key := "key"
	testSet := set.Set[string]{
		key: struct{}{},
	}
	assert.Assert(t, testSet.Contains(key))
}

func TestKeys(t *testing.T) {
	originalKeys := []int{1, 2, 3}
	testSet := set.Set[int]{}
	testSet.Add(originalKeys[0])
	testSet.Add(originalKeys[1])
	testSet.Add(originalKeys[2])
	resultKeys := testSet.Keys()
	sort.Ints(resultKeys)
	assert.DeepEqual(t, resultKeys, originalKeys)
}
