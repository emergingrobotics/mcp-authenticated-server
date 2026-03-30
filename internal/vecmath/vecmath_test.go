package vecmath

import (
	"math"
	"testing"
)

func TestL2Normalize(t *testing.T) {
	t.Run("normalizes to unit length", func(t *testing.T) {
		v := []float32{3, 4}
		L2Normalize(v)
		mag := math.Sqrt(float64(v[0])*float64(v[0]) + float64(v[1])*float64(v[1]))
		if math.Abs(mag-1.0) > 1e-6 {
			t.Errorf("expected magnitude ~1.0, got %f", mag)
		}
		if math.Abs(float64(v[0])-0.6) > 1e-6 {
			t.Errorf("expected v[0] ~0.6, got %f", v[0])
		}
		if math.Abs(float64(v[1])-0.8) > 1e-6 {
			t.Errorf("expected v[1] ~0.8, got %f", v[1])
		}
	})

	t.Run("zero vector unchanged", func(t *testing.T) {
		v := []float32{0, 0, 0}
		L2Normalize(v)
		for i, x := range v {
			if x != 0 {
				t.Errorf("expected v[%d]=0, got %f", i, x)
			}
		}
	})

	t.Run("already normalized vector", func(t *testing.T) {
		v := []float32{1, 0, 0}
		L2Normalize(v)
		if math.Abs(float64(v[0])-1.0) > 1e-6 {
			t.Errorf("expected v[0]=1.0, got %f", v[0])
		}
	})

	t.Run("high dimensional vector", func(t *testing.T) {
		v := make([]float32, 768)
		for i := range v {
			v[i] = float32(i) * 0.01
		}
		L2Normalize(v)
		var sum float64
		for _, x := range v {
			sum += float64(x) * float64(x)
		}
		if math.Abs(sum-1.0) > 1e-5 {
			t.Errorf("expected magnitude squared ~1.0, got %f", sum)
		}
	})
}

func TestDotProduct(t *testing.T) {
	t.Run("basic dot product", func(t *testing.T) {
		a := []float32{1, 2, 3}
		b := []float32{4, 5, 6}
		got := DotProduct(a, b)
		want := float32(32) // 1*4 + 2*5 + 3*6
		if got != want {
			t.Errorf("expected %f, got %f", want, got)
		}
	})

	t.Run("orthogonal vectors", func(t *testing.T) {
		a := []float32{1, 0}
		b := []float32{0, 1}
		got := DotProduct(a, b)
		if got != 0 {
			t.Errorf("expected 0, got %f", got)
		}
	})

	t.Run("panics on mismatched lengths", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic on mismatched lengths")
			}
		}()
		DotProduct([]float32{1, 2}, []float32{1, 2, 3})
	})
}

func TestCosineSimilarity(t *testing.T) {
	t.Run("identical normalized vectors", func(t *testing.T) {
		a := []float32{0.6, 0.8}
		L2Normalize(a)
		got := CosineSimilarity(a, a)
		if math.Abs(float64(got)-1.0) > 1e-6 {
			t.Errorf("expected ~1.0, got %f", got)
		}
	})

	t.Run("opposite vectors", func(t *testing.T) {
		a := []float32{1, 0}
		b := []float32{-1, 0}
		got := CosineSimilarity(a, b)
		if math.Abs(float64(got)+1.0) > 1e-6 {
			t.Errorf("expected ~-1.0, got %f", got)
		}
	})
}
