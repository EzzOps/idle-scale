package common

const (
	AnnotationEnabled    = "idle-scale.nous.io/enabled"
	AnnotationPort       = "idle-scale.nous.io/port"
	AnnotationIdleTimeout = "idle-scale.nous.io/idle-timeout"
	AnnotationIgnorePaths = "idle-scale.nous.io/ignore-paths"
	AnnotationStartupGrace = "idle-scale.nous.io/startup-grace"

	LabelManaged   = "idle-scale.nous.io/managed"
	LabelSentinel  = "idle-scale.nous.io/role"
	LabelDeployRef = "idle-scale.nous.io/deployment"

	SentinelExitCode = 42
	DefaultSentinelImage = "ghcr.io/ezzops/idle-scale-sentinel:latest"
)
