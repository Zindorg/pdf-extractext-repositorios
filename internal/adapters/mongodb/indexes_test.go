package mongodb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndexSpecsNames(t *testing.T) {
	var got []string
	for _, s := range indexSpecs() {
		got = append(got, s.name)
	}

	require.Equal(t, []string{
		idxDocumentID,
		idxChecksumAlive,
		idxFilename,
		idxCreatedAt,
		idxSummaryStatus,
	}, got)
}

func TestIndexModelsMatchSpecs(t *testing.T) {
	require.Len(t, indexModels(), len(indexSpecs()))
}