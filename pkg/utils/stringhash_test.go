package utils

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func TestHashFromString(t *testing.T) {
	assert := tassert.New(t)

	stringHash := HashFromString("some string")
	assert.NotZero(stringHash)

	emptyStringHash := HashFromString("")
	assert.Equal(emptyStringHash, uint64(14695981039346656037))

	assert.NotEqual(stringHash, emptyStringHash)
}
