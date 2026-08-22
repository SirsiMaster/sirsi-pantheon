package sne

const (
	interactiveMaxConcurrentRequests       = 1
	interactiveMaxQueuedRequests           = 8
	fleetMaxConcurrentRequests             = 1
	fleetMaxQueuedRequests                 = 32
	servingQueueDiscipline                 = "fifo"
	servingRequestTimeoutMS          int64 = 120000
)

func expectedServingPolicy(profile string) SupportServingPolicy {
	if profile == "fleet" {
		return SupportServingPolicy{Profile: profile, MaxConcurrentRequests: fleetMaxConcurrentRequests, MaxQueuedRequests: fleetMaxQueuedRequests, QueueDiscipline: servingQueueDiscipline, RequestTimeoutMS: servingRequestTimeoutMS}
	}
	return SupportServingPolicy{Profile: profile, MaxConcurrentRequests: interactiveMaxConcurrentRequests, MaxQueuedRequests: interactiveMaxQueuedRequests, QueueDiscipline: servingQueueDiscipline, RequestTimeoutMS: servingRequestTimeoutMS}
}
