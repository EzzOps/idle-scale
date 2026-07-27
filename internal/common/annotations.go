package common

const (
	// AnnotationEnabled marks a Deployment as managed by idle-scale.
	AnnotationEnabled = "idle-scale.nous.io/enabled"

	// AnnotationPort is the service port the sentinel should listen on.
	AnnotationPort = "idle-scale.nous.io/port"

	// AnnotationIdleTimeout is the idle duration before scaling to zero.
	AnnotationIdleTimeout = "idle-scale.nous.io/idle-timeout"

	// AnnotationIgnorePaths is a comma-separated list of HTTP paths to ignore.
	AnnotationIgnorePaths = "idle-scale.nous.io/ignore-paths"

	// AnnotationStartupGrace is the grace period before a new deployment is managed.
	AnnotationStartupGrace = "idle-scale.nous.io/startup-grace"

	// LabelManaged indicates a resource is managed by idle-scale.
	LabelManaged = "idle-scale.nous.io/managed"

	// LabelSentinel marks a pod as a sentinel.
	LabelSentinel = "idle-scale.nous.io/role"

	// LabelDeployRef references the deployment name.
	LabelDeployRef = "idle-scale.nous.io/deployment"

	// SentinelRoleValue is the value for the sentinel role label.
	SentinelRoleValue = "sentinel"

	// SentinelExitCode is the exit code for traffic detection.
	SentinelExitCode = 42

	// DefaultSentinelImage is the default sentinel container image.
	DefaultSentinelImage = "ghcr.io/ezzops/idle-scale-sentinel:latest"

	// DefaultPort is the default port sentinel listens on.
	DefaultPort = "8080"

	// DefaultIgnorePaths is the default set of ignored HTTP paths.
	DefaultIgnorePaths = "/healthz,/readyz,/livez,/metrics"
)
