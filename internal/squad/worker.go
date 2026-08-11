package squad

import "github.com/hibiken/asynq"

// TypeCompleteMissions is the periodic task that finishes timed away missions
// whose ReturnsAt has passed (grants rewards, returns squads to idle).
const TypeCompleteMissions = "squad:complete_missions"

// NewCompleteMissionsTask builds the recurring mission-completion task.
func NewCompleteMissionsTask() *asynq.Task {
	return asynq.NewTask(TypeCompleteMissions, nil)
}
