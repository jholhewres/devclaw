// Package memory – quantized_embedding.go implements uint8-quantized embeddings
// for memory-efficient vector search. Inspired by TurboQuant's asymmetric estimation:
// data is stored in uint8 (1 byte/dim), queries stay float32 for maximum precision.
//
// Compression: float32 (4 bytes/dim) → uint8 (1 byte/dim) = 4x reduction.
// Quality: correlation ≥ 0.98 with float32 cosine similarity on normalized embeddings.
package memory

import (
	"encoding/binary"
	"fmt"
	"math"
)

// QuantizedEmbedding stores a uint8-quantized embedding with scale/min for reconstruction.
// Uses the full [0, 255] range for maximum precision (8 effective bits).
type QuantizedEmbedding struct {
	Data   []uint8 // quantized values in [0, 255]
	Scale  float32 // (max - min) / 255; zero when all values are identical
	MinVal float32 // minimum value before quantization
	Dims   int     // original dimensionality
}

// QuantizeFloat32 converts a float32 embedding to uint8 quantized form.
// Uses affine quantization: q = round((v - min) / (max - min) * 255).
func QuantizeFloat32(vec []float32) QuantizedEmbedding {
	if len(vec) == 0 {
		return QuantizedEmbedding{Dims: 0}
	}

	// Find min/max in a single pass.
	minVal := vec[0]
	maxVal := vec[0]
	for _, v := range vec[1:] {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	span := maxVal - minVal
	var scale float32
	if span > 0 {
		scale = span / 255.0
	}

	data := make([]uint8, len(vec))
	if span > 0 {
		invSpan := 255.0 / float64(span)
		for i, v := range vec {
			normalized := float64(v-minVal) * invSpan
			data[i] = uint8(math.Round(normalized))
		}
	}
	// When span == 0 all data stays zero (all values identical).

	return QuantizedEmbedding{
		Data:   data,
		Scale:  scale,
		MinVal: minVal,
		Dims:   len(vec),
	}
}

// Dequantize reconstructs the float32 embedding from quantized form.
func (q QuantizedEmbedding) Dequantize() []float32 {
	out := make([]float32, q.Dims)
	for i := 0; i < q.Dims; i++ {
		out[i] = float32(q.Data[i])*q.Scale + q.MinVal
	}
	return out
}

// DotProduct computes the dot product between this quantized embedding and a
// float32 query vector. Uses the asymmetric estimator pattern: the query stays
// in full precision while the data is reconstructed on-the-fly from uint8.
//
// Mathematically: Σ (data[i]*scale + minVal) * query[i]
//
//	= scale * Σ data[i]*query[i]  +  minVal * Σ query[i]
//
// This avoids per-element dequantization by factoring out scale and minVal.
func (q QuantizedEmbedding) DotProduct(query []float32) float64 {
	if len(query) != q.Dims {
		return 0
	}

	var sumDQ float64 // Σ data[i] * query[i]
	var sumQ float64  // Σ query[i]

	for i := 0; i < q.Dims; i++ {
		qi := float64(query[i])
		sumDQ += float64(q.Data[i]) * qi
		sumQ += qi
	}

	return float64(q.Scale)*sumDQ + float64(q.MinVal)*sumQ
}

// CosineSimilarity computes cosine similarity between this quantized embedding
// and a float32 query. queryNorm is the precomputed L2 norm of the query
// (compute once per search, reuse across all candidates).
func (q QuantizedEmbedding) CosineSimilarity(query []float32, queryNorm float64) float64 {
	if len(query) != q.Dims || queryNorm == 0 {
		return 0
	}

	dot := q.DotProduct(query)

	// Compute norm of the dequantized embedding.
	// ||x||² = scale² * Σ data[i]² + 2*scale*minVal * Σ data[i] + dims * minVal²
	var sumD2 float64 // Σ data[i]²
	var sumD float64  // Σ data[i]
	for i := 0; i < q.Dims; i++ {
		d := float64(q.Data[i])
		sumD2 += d * d
		sumD += d
	}

	s := float64(q.Scale)
	m := float64(q.MinVal)
	normSq := s*s*sumD2 + 2*s*m*sumD + float64(q.Dims)*m*m

	if normSq <= 0 {
		return 0
	}

	return dot / (math.Sqrt(normSq) * queryNorm)
}

// ---------- Binary Serialization ----------

// binaryHeaderSize is the fixed header: 4 (dims) + 4 (scale) + 4 (minVal) = 12 bytes.
const binaryHeaderSize = 12

// MarshalBinary serializes the quantized embedding to a compact binary format:
// [dims:uint32LE][scale:float32LE][minVal:float32LE][data:uint8...]
func (q QuantizedEmbedding) MarshalBinary() ([]byte, error) {
	buf := make([]byte, binaryHeaderSize+q.Dims)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(q.Dims))
	binary.LittleEndian.PutUint32(buf[4:8], math.Float32bits(q.Scale))
	binary.LittleEndian.PutUint32(buf[8:12], math.Float32bits(q.MinVal))
	copy(buf[binaryHeaderSize:], q.Data)
	return buf, nil
}

// UnmarshalBinary deserializes a quantized embedding from binary format.
func (q *QuantizedEmbedding) UnmarshalBinary(data []byte) error {
	if len(data) < binaryHeaderSize {
		return fmt.Errorf("quantized embedding: binary too short (%d bytes)", len(data))
	}

	q.Dims = int(binary.LittleEndian.Uint32(data[0:4]))
	const maxEmbeddingDims = 65536
	if q.Dims > maxEmbeddingDims {
		return fmt.Errorf("quantized embedding: dims %d exceeds maximum %d", q.Dims, maxEmbeddingDims)
	}
	q.Scale = math.Float32frombits(binary.LittleEndian.Uint32(data[4:8]))
	q.MinVal = math.Float32frombits(binary.LittleEndian.Uint32(data[8:12]))

	expected := binaryHeaderSize + q.Dims
	if len(data) != expected {
		return fmt.Errorf("quantized embedding: expected %d bytes, got %d", expected, len(data))
	}

	q.Data = make([]uint8, q.Dims)
	copy(q.Data, data[binaryHeaderSize:])
	return nil
}

// ---------- Helpers ----------

// VectorNorm computes the L2 norm of a float32 vector. Useful for precomputing
// queryNorm before calling CosineSimilarity across many candidates.
func VectorNorm(v []float32) float64 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	return math.Sqrt(sum)
}
