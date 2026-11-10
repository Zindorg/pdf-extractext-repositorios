package mongodb

import (
	"regexp"
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestBuildMongoFilter_FilenameEscapesRegex(t *testing.T) {
	// Caracteres especiales de regex que deben escaparse
	testCases := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"file.txt", "file\\.txt"},
		{"a*b", "a\\*b"},
		{"a+b", "a\\+b"},
		{"a?b", "a\\?b"},
		{"a(b)c", "a\\(b\\)c"},
		{"a[b]c", "a\\[b\\]c"},
		{"a{b}c", "a\\{b\\}c"},
		{"a|b", "a\\|b"},
		{"a^b$c", "a\\^b\\$c"},
		{"a.b", "a\\.b"},
		{"*", "\\*"},
		{".*", "\\.\\*"},
		{"", ""},
		{"filename_with_underscores", "filename_with_underscores"},
		{"file-name", "file-name"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			filter := domain.ListFilter{}
			if tc.input != "" {
				filter.Filename = &tc.input
			}
			m := buildMongoFilter(filter)

			if tc.input == "" {
				require.NotContains(t, m, "metadata.filename")
				return
			}

			regexFilter, ok := m["metadata.filename"].(bson.M)
			require.True(t, ok, "debe haber filtro regex para filename")

			pattern, ok := regexFilter["$regex"].(string)
			require.True(t, ok, "patrón debe ser string")

			// Verificar que el patrón coincide con lo que esperamos (caracteres escapados)
			require.Equal(t, tc.expected, pattern)

			// Verificar que el patrón compilado coincide con el input original (sin regex)
			re, err := regexp.Compile("(?i)" + pattern)
			require.NoError(t, err)
			require.True(t, re.MatchString(tc.input), "el patrón debe hacer match con el input original")

			// Verificar que NO hace match con una variante que sí haría match si no se escapara
			// Solo para casos donde el input original es un regex válido y peligroso
			if tc.input != tc.expected && isValidRegex(tc.input) {
				reUnescaped, err := regexp.Compile("(?i)" + tc.input)
				if err == nil && reUnescaped.MatchString("acb") != re.MatchString("acb") {
					require.False(t, re.MatchString("acb"), "el patrón escapado NO debe hacer match con lo que haría el no escapado")
				}
			}
		})
	}
}

func isValidRegex(s string) bool {
	_, err := regexp.Compile("(?i)" + s)
	return err == nil
}

func TestBuildMongoFilter_FilenameNil(t *testing.T) {
	filter := domain.ListFilter{}
	filter.Filename = nil
	m := buildMongoFilter(filter)
	require.NotContains(t, m, "metadata.filename")
}
