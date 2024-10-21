package common

import "cmp"

func SelectSortArray[T cmp.Ordered](table []T) {
	if len(table) <= 1 {
		return
	}

	// Select Sort
	for i := 0; i < len(table)-1; i++ {
		jMin := i

		for j := i + 1; j < len(table); j++ {
			if table[j] < table[jMin] {
				jMin = j
				continue
			}
		}
		if jMin != i {
			tmp := table[i]
			table[i] = table[jMin]
			table[jMin] = tmp
		}
	}
}
