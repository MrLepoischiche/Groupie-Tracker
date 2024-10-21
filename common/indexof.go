package common

func IndexOf[T comparable](arr []T, match T) int {
	if len(arr) <= 0 {
		return -1
	}

	for i, elem := range arr {
		if elem == match {
			return i
		}
	}

	return -1
}
