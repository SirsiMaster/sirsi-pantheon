package rules

import "github.com/SirsiMaster/sirsi-anubis/internal/jackal"

// ═══════════════════════════════════════════
// AI / ML — Model caches, training artifacts
// ═══════════════════════════════════════════

// NewHuggingFaceCacheRule scans for HuggingFace downloaded models.
func NewHuggingFaceCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "huggingface_cache",
		displayName: "HuggingFace Models",
		category:    jackal.CategoryAI,
		description: "Downloaded model weights from HuggingFace Hub (often 2-20 GB each)",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.cache/huggingface/hub",
		},
	}
}

// NewOllamaModelsRule scans for Ollama local models.
func NewOllamaModelsRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "ollama_models",
		displayName: "Ollama Models",
		category:    jackal.CategoryAI,
		description: "Locally downloaded Ollama LLM models (often 4-70 GB each)",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.ollama/models",
		},
	}
}

// NewPyTorchCacheRule scans for PyTorch hub and model cache.
func NewPyTorchCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "pytorch_cache",
		displayName: "PyTorch Cache",
		category:    jackal.CategoryAI,
		description: "PyTorch hub models and compiled extensions",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.cache/torch",
		},
	}
}

// NewMLXCacheRule scans for Apple MLX model cache.
func NewMLXCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "mlx_cache",
		displayName: "Apple MLX Cache",
		category:    jackal.CategoryAI,
		description: "Apple MLX converted models and compilation cache",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/.cache/mlx",
			"~/Library/Caches/mlx",
		},
	}
}

// NewMetalShaderCacheRule scans for compiled Metal shader caches.
func NewMetalShaderCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "metal_shaders",
		displayName: "Metal Shader Cache",
		category:    jackal.CategoryAI,
		description: "Compiled Metal GPU shader caches",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Caches/com.apple.metal",
		},
	}
}

// NewTensorFlowCacheRule scans for TensorFlow cache.
func NewTensorFlowCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "tensorflow_cache",
		displayName: "TensorFlow Cache",
		category:    jackal.CategoryAI,
		description: "TensorFlow model cache and compiled ops",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.cache/tensorflow",
			"~/.keras/models",
		},
	}
}
