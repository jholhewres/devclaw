# Vector Search Evolution

Current implementation: in-memory brute-force cosine similarity over a `map[int64]vectorCacheEntry` cache, loaded from SQLite on startup and updated incrementally per-file. This scales well to ~10K chunks.

## Future Options

### 1. sqlite-vec (vec0 virtual table)

ANN search natively inside SQLite via the `sqlite-vec` extension.

- **Best for**: >50K chunks
- **Pros**: No external dependency, queries via SQL, disk-backed with mmap
- **Cons**: Requires fixed embedding dimensions at table creation (incompatible with multi-provider setups where dimensions vary), needs a migration script, C extension must be compiled per platform
- **Migration path**: Add `vec0` virtual table alongside `chunks`, sync on index, fallback to current brute-force if extension unavailable

### 2. Go-native HNSW (e.g. `coder/hnsw`)

Hierarchical Navigable Small World graph built in-process over the existing cache.

- **Best for**: 10K-100K chunks
- **Pros**: O(log N) approximate search, no schema change, pure Go, works with variable-dimension embeddings
- **Cons**: Higher memory usage (graph structure), rebuild needed on startup, tuning parameters (M, efConstruction)
- **Migration path**: Build HNSW index after `loadVectorCache`, use for `SearchVector`, keep brute-force as fallback for small caches

### 3. External Vector DB (Qdrant, Milvus, Weaviate)

Dedicated vector database as a sidecar or remote service.

- **Best for**: >100K chunks, distributed search, multi-tenant
- **Pros**: Purpose-built ANN, horizontal scaling, filtering, metadata indexing
- **Cons**: Runtime dependency (breaks "zero dependencies" goal), network latency, operational overhead
- **Migration path**: Abstract `SearchVector` behind interface, add Qdrant client as optional provider, keep SQLite as default
