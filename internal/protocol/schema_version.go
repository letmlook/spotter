package protocol

// SchemaVersion is bumped on any breaking change to DeviceInfo fields.
//
//   v1 (0.1.0): initial shape.
//   v2 (0.3.0): AgentIdentity.Auth *AuthInfo added (omitempty). Old
//        clients ignore it; new clients may show "auth required" badge
//        when the field is present and required.
//
// Clients MUST treat any unknown field as a no-op (the JSON encoder
// already does so). Agents MUST continue to honour v1-only clients
// by setting Auth = nil in their responses until the operator enables
// `auth.enabled = true` in agent.toml.
const SchemaVersion = 2
