package vecmath

import "math"

// L2Normalize normalizes the vector in-place to unit length.
// A zero-magnitude vector is left unchanged.
func L2Normalize(v []float32) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	norm := float32(math.Sqrt(sum))
	if norm == 0 {
		return
	}
	for i := range v {
		v[i] /= norm
	}
}

// DotProduct computes the dot product of two equal-length vectors.
// Panics if lengths differ.
func DotProduct(a, b []float32) float32 {
	if len(a) != len(b) {
		panic("vecmath: mismatched vector lengths")
	}
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// CosineSimilarity computes cosine similarity between two vectors.
// For L2-normalized vectors this equals their dot product.
func CosineSimilarity(a, b []float32) float32 {
	return DotProduct(a, b)
}
