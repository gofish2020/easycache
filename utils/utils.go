package utils

func IsPowerOfTwo(number int) bool {
	return (number != 0) && (number&(number-1)) == 0
}
