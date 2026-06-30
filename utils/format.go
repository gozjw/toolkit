package utils

import (
	"fmt"
	"strconv"
)

func FormatBytesIEC(size int64) string {
	if size < 1024 {
		return strconv.FormatInt(size, 10) + "B"
	}

	units := [...]string{"K", "M", "G", "T"}

	v := float64(size)
	i := 0

	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}

	return fmt.Sprintf("%.2f%s", v, units[i-1])
}
