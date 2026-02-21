// Package copilot – team_types.go defines types for the team/agent management system.
// This enables persistent agents, shared memory, and inter-agent communication.
package copilot

import (
	"time"
)

// ─── Team ───

// Team represents a group of agents working together with shared context.
type Team struct {
	// ID is the unique team identifier.
	ID string `json:"id" yaml:"id"`

	// Name is the human-readable team name.
	Name string `json:"name" yaml:"name"`

	// Description explains the team's purpose.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// OwnerJID is the user who owns this team.
	OwnerJID string `json:"owner_jid" yaml:"owner_jid"`

	// DefaultModel is the default LLM model for agents in this team.
	DefaultModel string `json:"default_model,omitempty" yaml:"default_model,omitempty"`

	// WorkspacePath is the team's workspace directory for files/memory.
	WorkspacePath string `json:"workspace_path,omitempty" yaml:"workspace_path,omitempty"`

	// CreatedAt is when the team was created.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// Enabled indicates if the team is active.
	Enabled bool `json:"enabled" yaml:"enabled"`
}

// ─── Persistent Agent ───

// AgentStatus represents the current state of a persistent agent.
type AgentStatus string

const (
	AgentStatusIdle     AgentStatus = "idle"
	AgentStatusActive   AgentStatus = "active"
	AgentStatusBlocked  AgentStatus = "blocked"
	AgentStatusStopped  AgentStatus = "stopped"
	AgentStatusError    AgentStatus = "error"
)

// AgentLevel defines the autonomy level of an agent.
type AgentLevel string

const (
	AgentLevelIntern     AgentLevel = "intern"     // Needs approval for actions
	AgentLevelSpecialist AgentLevel = "specialist" // Autonomous in domain
	AgentLevelLead       AgentLevel = "lead"       // Can delegate to others
)

// PersistentAgent represents a long-lived agent with a specific role.
type PersistentAgent struct {
	// ID is the unique agent identifier (used for @mentions).
	ID string `json:"id" yaml:"id"`

	// Name is the human-readable name (e.g., "Jarvis", "Loki").
	Name string `json:"name" yaml:"name"`

	// Role describes what this agent does (e.g., "Writer", "Researcher").
	Role string `json:"role" yaml:"role"`

	// TeamID is the team this agent belongs to.
	TeamID string `json:"team_id" yaml:"team_id"`

	// Level defines the agent's autonomy level.
	Level AgentLevel `json:"level" yaml:"level"`

	// Status is the current agent state.
	Status AgentStatus `json:"status" yaml:"status"`

	// Personality is the SOUL.md content for this agent.
	Personality string `json:"personality,omitempty" yaml:"personality,omitempty"`

	// Instructions are additional operating instructions.
	Instructions string `json:"instructions,omitempty" yaml:"instructions,omitempty"`

	// Model is the LLM model for this agent (empty = team default).
	Model string `json:"model,omitempty" yaml:"model,omitempty"`

	// Skills are the skill names this agent can use.
	Skills []string `json:"skills,omitempty" yaml:"skills,omitempty"`

	// SessionID is the internal session identifier.
	SessionID string `json:"session_id,omitempty" yaml:"-"`

	// CurrentTaskID is the task this agent is currently working on.
	CurrentTaskID string `json:"current_task_id,omitempty" yaml:"-"`

	// HeartbeatSchedule is the cron expression for heartbeats (default: "*/15 * * * *").
	HeartbeatSchedule string `json:"heartbeat_schedule,omitempty" yaml:"heartbeat_schedule,omitempty"`

	// CreatedAt is when the agent was created.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// LastActiveAt is when the agent was last active.
	LastActiveAt time.Time `json:"last_active_at,omitempty" yaml:"last_active_at,omitempty"`

	// LastHeartbeatAt is when the last heartbeat ran.
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at,omitempty" yaml:"last_heartbeat_at,omitempty"`

	// HeartbeatJobID is the scheduler job ID for heartbeats.
	HeartbeatJobID string `json:"heartbeat_job_id,omitempty" yaml:"-"`
}

// ─── Team Tasks ───

// TaskStatus represents the state of a team task.
type TaskStatus string

const (
	TaskStatusInbox     TaskStatus = "inbox"
	TaskStatusAssigned  TaskStatus = "assigned"
	TaskStatusProgress  TaskStatus = "in_progress"
	TaskStatusReview    TaskStatus = "review"
	TaskStatusDone      TaskStatus = "done"
	TaskStatusBlocked   TaskStatus = "blocked"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// TeamTask represents a shared task that agents can work on.
type TeamTask struct {
	// ID is the unique task identifier.
	ID string `json:"id" yaml:"id"`

	// TeamID is the team this task belongs to.
	TeamID string `json:"team_id" yaml:"team_id"`

	// Title is the short task description.
	Title string `json:"title" yaml:"title"`

	// Description is the detailed task description.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Status is the current task state.
	Status TaskStatus `json:"status" yaml:"status"`

	// Assignees are the agent IDs assigned to this task.
	Assignees []string `json:"assignees,omitempty" yaml:"assignees,omitempty"`

	// Priority is the task priority (1-5, 1=highest).
	Priority int `json:"priority,omitempty" yaml:"priority,omitempty"`

	// Labels are arbitrary tags for organization.
	Labels []string `json:"labels,omitempty" yaml:"labels,omitempty"`

	// CreatedBy is the agent or user who created the task.
	CreatedBy string `json:"created_by" yaml:"created_by"`

	// CreatedAt is when the task was created.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// UpdatedAt is when the task was last modified.
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`

	// CompletedAt is when the task was marked done.
	CompletedAt *time.Time `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`

	// BlockedReason explains why the task is blocked.
	BlockedReason string `json:"blocked_reason,omitempty" yaml:"blocked_reason,omitempty"`
}

// ─── Team Messages ───

// TeamMessage represents a message in a task thread or general discussion.
type TeamMessage struct {
	// ID is the unique message identifier.
	ID string `json:"id" yaml:"id"`

	// TeamID is the team this message belongs to.
	TeamID string `json:"team_id" yaml:"team_id"`

	// ThreadID is the task ID or thread identifier (empty = general).
	ThreadID string `json:"thread_id,omitempty" yaml:"thread_id,omitempty"`

	// FromAgent is the agent ID that sent the message.
	FromAgent string `json:"from_agent" yaml:"from_agent"`

	// FromUser is the user JID if sent by a human (optional).
	FromUser string `json:"from_user,omitempty" yaml:"from_user,omitempty"`

	// Content is the message text.
	Content string `json:"content" yaml:"content"`

	// Mentions are agent IDs mentioned in the message (@agent_id).
	Mentions []string `json:"mentions,omitempty" yaml:"mentions,omitempty"`

	// CreatedAt is when the message was sent.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// Delivered indicates if mentioned agents have received this.
	Delivered bool `json:"delivered" yaml:"delivered"`
}

// ─── Team Facts (Shared Memory) ───

// TeamFact represents a shared fact in team memory.
type TeamFact struct {
	// ID is the unique fact identifier.
	ID string `json:"id" yaml:"id"`

	// TeamID is the team this fact belongs to.
	TeamID string `json:"team_id" yaml:"team_id"`

	// Key is the fact key/label.
	Key string `json:"key" yaml:"key"`

	// Value is the fact content.
	Value string `json:"value" yaml:"value"`

	// Author is the agent or user who created this fact.
	Author string `json:"author" yaml:"author"`

	// CreatedAt is when the fact was created.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// UpdatedAt is when the fact was last modified.
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`
}

// ─── Agent Mailbox ───

// PendingMessage is a message waiting to be delivered to an agent.
type PendingMessage struct {
	// ID is the unique message identifier.
	ID string `json:"id" yaml:"id"`

	// ToAgent is the destination agent ID.
	ToAgent string `json:"to_agent" yaml:"to_agent"`

	// FromAgent is the source agent ID (empty = from user).
	FromAgent string `json:"from_agent,omitempty" yaml:"from_agent,omitempty"`

	// FromUser is the user JID if sent by a human.
	FromUser string `json:"from_user,omitempty" yaml:"from_user,omitempty"`

	// Content is the message text.
	Content string `json:"content" yaml:"content"`

	// ThreadID is the related task/thread ID.
	ThreadID string `json:"thread_id,omitempty" yaml:"thread_id,omitempty"`

	// CreatedAt is when the message was created.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// Delivered indicates if the message was delivered.
	Delivered bool `json:"delivered" yaml:"delivered"`

	// DeliveredAt is when the message was delivered.
	DeliveredAt *time.Time `json:"delivered_at,omitempty" yaml:"delivered_at,omitempty"`
}

// ─── Activity Feed ───

// ActivityType represents the type of team activity.
type ActivityType string

const (
	ActivityTaskCreated    ActivityType = "task_created"
	ActivityTaskUpdated    ActivityType = "task_updated"
	ActivityTaskCompleted  ActivityType = "task_completed"
	ActivityTaskAssigned   ActivityType = "task_assigned"
	ActivityMessageSent    ActivityType = "message_sent"
	ActivityMention        ActivityType = "mention"
	ActivityFactCreated    ActivityType = "fact_created"
	ActivityAgentActive    ActivityType = "agent_active"
	ActivityAgentIdle      ActivityType = "agent_idle"
	ActivityDocumentCreated ActivityType = "document_created"
	ActivityDocumentUpdated ActivityType = "document_updated"
	ActivitySubscribed     ActivityType = "subscribed"
)

// TeamActivity represents an entry in the activity feed.
type TeamActivity struct {
	// ID is the unique activity identifier.
	ID string `json:"id" yaml:"id"`

	// TeamID is the team this activity belongs to.
	TeamID string `json:"team_id" yaml:"team_id"`

	// Type is the activity type.
	Type ActivityType `json:"type" yaml:"type"`

	// AgentID is the agent that performed the activity.
	AgentID string `json:"agent_id,omitempty" yaml:"agent_id,omitempty"`

	// Message is a human-readable description.
	Message string `json:"message" yaml:"message"`

	// RelatedID is the related entity ID (task, message, etc.).
	RelatedID string `json:"related_id,omitempty" yaml:"related_id,omitempty"`

	// CreatedAt is when the activity occurred.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
}

// ─── Team Documents ───

// DocumentType represents the type of team document.
type DocumentType string

const (
	DocumentTypeDeliverable DocumentType = "deliverable"
	DocumentTypeResearch    DocumentType = "research"
	DocumentTypeProtocol    DocumentType = "protocol"
	DocumentTypeNotes       DocumentType = "notes"
)

// TeamDocument represents a deliverable or artifact linked to a task.
type TeamDocument struct {
	// ID is the unique document identifier.
	ID string `json:"id" yaml:"id"`

	// TeamID is the team this document belongs to.
	TeamID string `json:"team_id" yaml:"team_id"`

	// TaskID is the task this document is linked to (optional).
	TaskID string `json:"task_id,omitempty" yaml:"task_id,omitempty"`

	// Title is the document title.
	Title string `json:"title" yaml:"title"`

	// DocType is the document type.
	DocType DocumentType `json:"doc_type" yaml:"doc_type"`

	// Content is the document content (markdown, code, etc.).
	Content string `json:"content" yaml:"content"`

	// Format is the content format (markdown, code, json).
	Format string `json:"format" yaml:"format"`

	// FilePath is an optional link to a workspace file.
	FilePath string `json:"file_path,omitempty" yaml:"file_path,omitempty"`

	// Version is the document version number.
	Version int `json:"version" yaml:"version"`

	// Author is the agent or user who created the document.
	Author string `json:"author" yaml:"author"`

	// CreatedAt is when the document was created.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// UpdatedAt is when the document was last modified.
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`
}

// ─── Thread Subscriptions ───

// SubscriptionReason represents how an agent was subscribed to a thread.
type SubscriptionReason string

const (
	SubscriptionAuto      SubscriptionReason = "auto"      // Automatic (commented)
	SubscriptionManual    SubscriptionReason = "manual"    // Explicitly subscribed
	SubscriptionMentioned SubscriptionReason = "mentioned" // Was @mentioned
	SubscriptionAssigned  SubscriptionReason = "assigned"  // Was assigned to task
)

// ThreadSubscription represents an agent's subscription to a thread.
type ThreadSubscription struct {
	// ID is the unique subscription identifier.
	ID string `json:"id" yaml:"id"`

	// TeamID is the team this subscription belongs to.
	TeamID string `json:"team_id" yaml:"team_id"`

	// ThreadID is the task or thread identifier.
	ThreadID string `json:"thread_id" yaml:"thread_id"`

	// AgentID is the subscribed agent.
	AgentID string `json:"agent_id" yaml:"agent_id"`

	// SubscribedAt is when the subscription was created.
	SubscribedAt time.Time `json:"subscribed_at" yaml:"subscribed_at"`

	// Reason is how the agent was subscribed.
	Reason SubscriptionReason `json:"reason" yaml:"reason"`
}

// ─── Agent Working State (WORKING.md) ───

// AgentWorkingState represents an agent's current work state.
type AgentWorkingState struct {
	// AgentID is the agent this state belongs to.
	AgentID string `json:"agent_id" yaml:"agent_id"`

	// TeamID is the team the agent belongs to.
	TeamID string `json:"team_id" yaml:"team_id"`

	// CurrentTaskID is the task the agent is currently working on.
	CurrentTaskID string `json:"current_task_id,omitempty" yaml:"current_task_id,omitempty"`

	// Status is the work status (idle, working, blocked, waiting).
	Status string `json:"status" yaml:"status"`

	// NextSteps describes what the agent plans to do next (markdown).
	NextSteps string `json:"next_steps,omitempty" yaml:"next_steps,omitempty"`

	// Context holds additional context for resuming work.
	Context string `json:"context,omitempty" yaml:"context,omitempty"`

	// UpdatedAt is when the state was last updated.
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`
}
