package utils

import "hash/fnv"

// HashFromString calculates an FNV-1 hash from a given string,
// returns it as a uint64
func HashFromString(s string) uint64 {
	h := fnv.New64()
	_, err := h.Write([]byte(s))
	if err != nil {
		log.Error().Err(err).Msgf("Failed to write hash for %s", s)
	}

	return h.Sum64()
}
