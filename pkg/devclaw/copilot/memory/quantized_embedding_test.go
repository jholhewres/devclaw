package memory

import (
	"math"
	"math/rand"
	"testing"
)

// ---------- QuantizeFloat32 ----------

func TestQuantizeFloat32_Roundtrip(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	dims := 1536
	vec := makeRandomVector(rng, dims)

	q := QuantizeFloat32(vec)
	reconstructed := q.Dequantize()

	if len(reconstructed) != dims {
		t.Fatalf("expected %d dims, got %d", dims, len(reconstructed))
	}

	// Check max per-dimension error.
	var maxErr float64
	var mse float64
	for i := range vec {
		diff := math.Abs(float64(vec[i]) - float64(reconstructed[i]))
		if diff > maxErr {
			maxErr = diff
		}
		mse += diff * diff
	}
	mse /= float64(dims)

	// For normalized embeddings (range ~[-0.1, 0.1]), max error should be
	// well under 0.02 per dimension.
	if maxErr > 0.02 {
		t.Errorf("max per-dimension error %.6f exceeds 0.02", maxErr)
	}
	t.Logf("roundtrip MSE=%.8f, maxErr=%.6f", mse, maxErr)
}

func TestQuantizeFloat32_FullRange(t *testing.T) {
	// Verify the full [0, 255] range is used, not [0, 127].
	vec := []float32{-1.0, 0.0, 1.0}
	q := QuantizeFloat32(vec)

	if q.Data[0] != 0 {
		t.Errorf("min value should quantize to 0, got %d", q.Data[0])
	}
	if q.Data[2] != 255 {
		t.Errorf("max value should quantize to 255, got %d", q.Data[2])
	}
	if q.Data[1] != 128 { // midpoint: round(0.5 * 255) = round(127.5) = 128
		t.Errorf("midpoint should quantize to 128, got %d", q.Data[1])
	}
}

func TestQuantizeFloat32_AllSameValue(t *testing.T) {
	vec := []float32{0.5, 0.5, 0.5, 0.5}
	q := QuantizeFloat32(vec)

	if q.Scale != 0 {
		t.Errorf("scale should be 0 for constant vector, got %f", q.Scale)
	}
	for i, v := range q.Data {
		if v != 0 {
			t.Errorf("data[%d] should be 0 for constant vector, got %d", i, v)
		}
	}

	reconstructed := q.Dequantize()
	for i, v := range reconstructed {
		if v != 0.5 {
			t.Errorf("reconstructed[%d] should be 0.5, got %f", i, v)
		}
	}
}

func TestQuantizeFloat32_Empty(t *testing.T) {
	q := QuantizeFloat32(nil)
	if q.Dims != 0 {
		t.Errorf("expected 0 dims for nil input, got %d", q.Dims)
	}
	if len(q.Dequantize()) != 0 {
		t.Error("dequantize of empty should return empty")
	}
}

// ---------- CosineSimilarity ----------

func TestCosineSimilarity_Quantized_vs_Float32(t *testing.T) {
	rng := rand.New(rand.NewSource(123))
	dims := 1536
	nVectors := 100

	query := makeRandomVector(rng, dims)
	queryNorm := VectorNorm(query)

	var sumDiffSq float64
	var sumRef float64
	var sumRefSq float64
	var maxDiff float64

	for i := 0; i < nVectors; i++ {
		vec := makeRandomVector(rng, dims)

		// Float32 reference.
		refSim := cosineSimilarity(query, vec)

		// Quantized.
		q := QuantizeFloat32(vec)
		qSim := q.CosineSimilarity(query, queryNorm)

		diff := math.Abs(refSim - qSim)
		if diff > maxDiff {
			maxDiff = diff
		}
		sumDiffSq += (refSim - qSim) * (refSim - qSim)
		sumRef += refSim
		sumRefSq += refSim * refSim
	}

	rmse := math.Sqrt(sumDiffSq / float64(nVectors))

	// Compute Pearson correlation.
	// We need the quantized sums too.
	rng2 := rand.New(rand.NewSource(123))
	query2 := makeRandomVector(rng2, dims)
	queryNorm2 := VectorNorm(query2)

	refs := make([]float64, nVectors)
	quants := make([]float64, nVectors)
	for i := 0; i < nVectors; i++ {
		vec := makeRandomVector(rng2, dims)
		refs[i] = cosineSimilarity(query2, vec)
		q := QuantizeFloat32(vec)
		quants[i] = q.CosineSimilarity(query2, queryNorm2)
	}
	corr := pearsonCorrelation(refs, quants)

	t.Logf("RMSE=%.6f, maxDiff=%.6f, correlation=%.6f", rmse, maxDiff, corr)

	if corr < 0.98 {
		t.Errorf("correlation %.4f < 0.98 threshold", corr)
	}
	if maxDiff > 0.05 {
		t.Errorf("max similarity difference %.4f > 0.05", maxDiff)
	}
}

func TestCosineSimilarity_DimensionMismatch(t *testing.T) {
	q := QuantizeFloat32([]float32{1, 2, 3})
	sim := q.CosineSimilarity([]float32{1, 2}, 1.0)
	if sim != 0 {
		t.Errorf("expected 0 for dimension mismatch, got %f", sim)
	}
}

func TestCosineSimilarity_ZeroQueryNorm(t *testing.T) {
	q := QuantizeFloat32([]float32{1, 2, 3})
	sim := q.CosineSimilarity([]float32{1, 2, 3}, 0)
	if sim != 0 {
		t.Errorf("expected 0 for zero queryNorm, got %f", sim)
	}
}

// ---------- DotProduct ----------

func TestDotProduct_Quantized_vs_Float32(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	dims := 1536

	query := makeRandomVector(rng, dims)
	vec := makeRandomVector(rng, dims)

	// Float32 reference dot product.
	var refDot float64
	for i := range query {
		refDot += float64(query[i]) * float64(vec[i])
	}

	q := QuantizeFloat32(vec)
	qDot := q.DotProduct(query)

	relErr := math.Abs(refDot-qDot) / math.Abs(refDot)
	t.Logf("refDot=%.6f, qDot=%.6f, relErr=%.6f", refDot, qDot, relErr)

	if relErr > 0.05 {
		t.Errorf("relative dot product error %.4f > 0.05", relErr)
	}
}

// ---------- Binary Serialization ----------

func TestMarshalBinary_Roundtrip(t *testing.T) {
	rng := rand.New(rand.NewSource(77))
	vec := makeRandomVector(rng, 1536)
	original := QuantizeFloat32(vec)

	data, err := original.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	expectedSize := binaryHeaderSize + 1536
	if len(data) != expectedSize {
		t.Fatalf("expected %d bytes, got %d", expectedSize, len(data))
	}

	var restored QuantizedEmbedding
	if err := restored.UnmarshalBinary(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.Dims != original.Dims {
		t.Errorf("dims: %d != %d", restored.Dims, original.Dims)
	}
	if restored.Scale != original.Scale {
		t.Errorf("scale: %f != %f", restored.Scale, original.Scale)
	}
	if restored.MinVal != original.MinVal {
		t.Errorf("minVal: %f != %f", restored.MinVal, original.MinVal)
	}
	for i := range original.Data {
		if restored.Data[i] != original.Data[i] {
			t.Errorf("data[%d]: %d != %d", i, restored.Data[i], original.Data[i])
			break
		}
	}
}

func TestUnmarshalBinary_TooShort(t *testing.T) {
	var q QuantizedEmbedding
	err := q.UnmarshalBinary([]byte{1, 2, 3})
	if err == nil {
		t.Error("expected error for too-short data")
	}
}

func TestUnmarshalBinary_WrongSize(t *testing.T) {
	var q QuantizedEmbedding
	// Header says 100 dims but only provide 50 bytes of data.
	buf := make([]byte, binaryHeaderSize+50)
	buf[0] = 100 // dims = 100
	err := q.UnmarshalBinary(buf)
	if err == nil {
		t.Error("expected error for size mismatch")
	}
}

// ---------- VectorNorm ----------

func TestVectorNorm(t *testing.T) {
	norm := VectorNorm([]float32{3, 4})
	if math.Abs(norm-5.0) > 1e-6 {
		t.Errorf("expected 5.0, got %f", norm)
	}
}

// ---------- Benchmarks ----------

func BenchmarkQuantizeFloat32_1536(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	vec := makeRandomVector(rng, 1536)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		QuantizeFloat32(vec)
	}
}

func BenchmarkCosineSimilarity_Float32_1536(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	query := makeRandomVector(rng, 1536)
	vec := makeRandomVector(rng, 1536)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cosineSimilarity(query, vec)
	}
}

func BenchmarkCosineSimilarity_Quantized_1536(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	query := makeRandomVector(rng, 1536)
	vec := makeRandomVector(rng, 1536)
	q := QuantizeFloat32(vec)
	queryNorm := VectorNorm(query)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.CosineSimilarity(query, queryNorm)
	}
}

// ---------- Helpers ----------

func makeRandomVector(rng *rand.Rand, dims int) []float32 {
	vec := make([]float32, dims)
	for i := range vec {
		vec[i] = float32(rng.NormFloat64()) * 0.05 // typical embedding range
	}
	return vec
}

func pearsonCorrelation(x, y []float64) float64 {
	n := float64(len(x))
	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := range x {
		sumX += x[i]
		sumY += y[i]
		sumXY += x[i] * y[i]
		sumX2 += x[i] * x[i]
		sumY2 += y[i] * y[i]
	}
	num := n*sumXY - sumX*sumY
	den := math.Sqrt((n*sumX2 - sumX*sumX) * (n*sumY2 - sumY*sumY))
	if den == 0 {
		return 0
	}
	return num / den
}
