package model

import (
	"strconv"
	"strings"

	"golang.org/x/exp/constraints"
)

// Series is a time series of values
type Series[T constraints.Ordered] []T

// Values returns the values of the series
func (s Series[T]) Values() []T {
	return s
}

// Length returns the number of values in the series
func (s Series[T]) Length() int {
	return len(s)
}

// Last returns the last value of the series given a past index position
func (s Series[T]) Last(position int) T {
	return s[len(s)-1-position]
}

// LastValues returns the last values of the series given a size
func (s Series[T]) LastValues(size int) []T {
	if l := len(s); l > size {
		return s[l-size:]
	}
	return s
}

func (s Series[T]) GetLastValues(size int, reversalStart int) []T {
	l := len(s)
	if l > size+reversalStart {
		return s[l-size-reversalStart : l-reversalStart]
	}
	return s[:l-reversalStart]
}

// Crossover returns true if the last value of the series is greater than the last value of the reference series
func (s Series[T]) Crossover(ref Series[T], stopIndex int) bool {
	return s.Last(stopIndex) > ref.Last(stopIndex) && s.Last(1+stopIndex) <= ref.Last(1+stopIndex)
}

// Crossunder returns true if the last value of the series is less than the last value of the reference series
func (s Series[T]) Crossunder(ref Series[T], stopIndex int) bool {
	return s.Last(stopIndex) <= ref.Last(stopIndex) && s.Last(1+stopIndex) > ref.Last(1+stopIndex)
}

// Cross returns true if the last value of the series is greater than the last value of the
// reference series or less than the last value of the reference series
func (s Series[T]) Cross(ref Series[T], stopIndex int) bool {
	return s.Crossover(ref, stopIndex) || s.Crossunder(ref, stopIndex)
}

// NumDecPlaces returns the number of decimal places of a float64
func NumDecPlaces(v float64) int64 {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	i := strings.IndexByte(s, '.')
	if i > -1 {
		return int64(len(s) - i - 1)
	}
	return 0
}
