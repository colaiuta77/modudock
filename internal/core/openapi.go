// 인증과 코어 API의 실제 요청 및 응답 계약을 명세로 제공한다.
package core

import (
	"strings"
)

type apiOperation struct{ path, method, body, response, status, role, scope string }

func schemaRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}
func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
func arraySchema(item any) map[string]any { return map[string]any{"type": "array", "items": item} }
func (s *Server) apiDocument() map[string]any {
	text := map[string]any{"type": "string"}
	integer := map[string]any{"type": "integer", "minimum": 0}
	boolean := map[string]any{"type": "boolean"}
	date := map[string]any{"type": "string", "format": "date-time"}
	role := map[string]any{"type": "string", "enum": []string{"admin", "operator", "viewer"}}
	password := map[string]any{"type": "string", "minLength": 12, "maxLength": 1024, "writeOnly": true}
	username := map[string]any{"type": "string", "pattern": "^[A-Za-z0-9_.-]{1,64}$"}
	user := objectSchema(map[string]any{"id": text, "username": username, "role": role, "is_active": boolean, "created_at": date}, "id", "username", "role", "is_active", "created_at")
	scopes := []string{"status:read", "plugins:read", "plugins:write", "runs:read", "runs:write", "schedules:read", "schedules:write", "logs:read", "users:write", "tokens:write"}
	scopeArray := map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": scopes}, "minItems": 1, "uniqueItems": true}
	schemas := map[string]any{
		"Error":          objectSchema(map[string]any{"error": objectSchema(map[string]any{"code": text, "message": text}, "code", "message")}, "error"),
		"User":           user,
		"Login":          objectSchema(map[string]any{"username": username, "password": map[string]any{"type": "string", "maxLength": 1024, "writeOnly": true}}, "username", "password"),
		"Auth":           objectSchema(map[string]any{"user": schemaRef("User"), "csrf_token": text}, "user", "csrf_token"),
		"PasswordChange": objectSchema(map[string]any{"current_password": map[string]any{"type": "string", "maxLength": 1024, "writeOnly": true}, "new_password": password}, "current_password", "new_password"),
		"UserCreate":     objectSchema(map[string]any{"username": username, "password": password, "role": role}, "username", "password", "role"),
		"UserUpdate":     objectSchema(map[string]any{"username": username, "password": password, "role": role, "is_active": boolean}, "username", "role", "is_active"),
		"TokenCreate":    objectSchema(map[string]any{"name": map[string]any{"type": "string", "minLength": 1, "maxLength": 100}, "scopes": scopeArray, "expires_at": date}, "name", "scopes", "expires_at"),
		"TokenMetadata":  objectSchema(map[string]any{"id": text, "name": text, "scopes": scopeArray, "expires_at": date, "created_at": date}, "id", "name", "scopes", "expires_at", "created_at"),
		"TokenCreated":   objectSchema(map[string]any{"token": text, "metadata": schemaRef("TokenMetadata")}, "token", "metadata"),
		"UserList":       objectSchema(map[string]any{"items": arraySchema(schemaRef("User")), "total": integer}, "items", "total"),
		"TokenList":      objectSchema(map[string]any{"items": arraySchema(schemaRef("TokenMetadata")), "total": integer}, "items", "total"),
		"Status":         objectSchema(map[string]any{"version": text, "started_at": date, "plugins_active": integer, "runs_queued": integer, "runs_running": integer, "runs_succeeded": integer, "runs_failed": integer}, "version", "started_at", "plugins_active", "runs_queued", "runs_running", "runs_succeeded", "runs_failed"),
		"Health":         objectSchema(map[string]any{"status": map[string]any{"type": "string", "const": "ok"}}, "status"),
	}
	logProperties := map[string]any{}
	logFields := []string{"timestamp", "level", "message", "component", "plugin_id", "task_id", "run_id", "user_id", "request_id"}
	for _, field := range logFields {
		logProperties[field] = text
	}
	logProperties["timestamp"] = date
	schemas["Log"] = objectSchema(logProperties, logFields...)
	schemas["LogList"] = objectSchema(map[string]any{"items": arraySchema(schemaRef("Log")), "total": integer}, "items", "total")
	operations := []apiOperation{{"/auth/login", "post", "Login", "Auth", "200", "", ""}, {"/auth/me", "get", "", "Auth", "200", "viewer", ""}, {"/auth/logout", "post", "", "", "204", "viewer", ""}, {"/auth/password", "put", "PasswordChange", "", "204", "viewer", "users:write"}, {"/users", "get", "", "UserList", "200", "admin", "users:write"}, {"/users", "post", "UserCreate", "User", "201", "admin", "users:write"}, {"/users/{id}", "put", "UserUpdate", "User", "200", "admin", "users:write"}, {"/users/{id}", "delete", "", "", "204", "admin", "users:write"}, {"/tokens", "get", "", "TokenList", "200", "admin", "tokens:write"}, {"/tokens", "post", "TokenCreate", "TokenCreated", "201", "admin", "tokens:write"}, {"/tokens/{id}", "delete", "", "", "204", "admin", "tokens:write"}, {"/status", "get", "", "Status", "200", "viewer", "status:read"}, {"/logs", "get", "", "LogList", "200", "operator", "logs:read"}, {"/events", "get", "", "", "200", "viewer", "status:read"}, {"/openapi.json", "get", "", "", "200", "viewer", ""}, {"/healthz", "get", "", "Health", "200", "", ""}}
	operations = append(operations, pluginAPISchemas(schemas)...)
	operations = append(operations, securityAPISchemas(schemas)...)
	paths := map[string]any{}
	for _, definition := range operations {
		success := map[string]any{"description": "Success"}
		if definition.response != "" {
			success["content"] = map[string]any{"application/json": map[string]any{"schema": schemaRef(definition.response)}}
		}
		responses := map[string]any{definition.status: success}
		if definition.path == "/auth/login" {
			responses["202"] = map[string]any{"description": "Second factor required; preauthentication cookies are not a session", "content": map[string]any{"application/json": map[string]any{"schema": schemaRef("MFAPending")}}}
		}
		for _, code := range []string{"400", "401", "403", "404", "405", "409", "413", "415", "429", "500", "502", "503"} {
			responses[code] = map[string]any{"description": "API error", "content": map[string]any{"application/json": map[string]any{"schema": schemaRef("Error")}}}
		}
		operation := map[string]any{"responses": responses, "x-minimum-role": definition.role, "x-required-scope": definition.scope}
		if definition.role == "" {
			operation["security"] = []any{}
		}
		if definition.scope == "session-only" {
			operation["security"] = []map[string][]string{{"session": {}}}
			operation["x-required-scope"] = ""
		}
		if definition.path == "/healthz" {
			operation["servers"] = []map[string]string{{"url": "/"}}
		}
		if definition.body != "" {
			operation["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": schemaRef(definition.body)}}}
		}
		parameters := []any{}
		for _, part := range strings.Split(definition.path, "/") {
			if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
				parameters = append(parameters, map[string]any{"name": strings.Trim(part, "{}"), "in": "path", "required": true, "schema": text})
			}
		}
		if definition.path == "/plugins/{id}/tasks/{task_id}/runs" {
			parameters = append(parameters, map[string]any{"name": "Idempotency-Key", "in": "header", "description": "Requester-scoped key; identical payload returns the same run, different payload returns 409", "schema": map[string]any{"type": "string", "maxLength": 200}})
		}
		if definition.method == "get" && (definition.path == "/users" || definition.path == "/tokens" || definition.path == "/logs" || definition.response == "PluginList" || definition.response == "ReleaseList" || definition.response == "MenuList" || definition.response == "RunList" || definition.response == "ScheduleList") {
			parameters = append(parameters, map[string]any{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "default": 50, "minimum": 1, "maximum": 200}}, map[string]any{"name": "offset", "in": "query", "schema": map[string]any{"type": "integer", "default": 0, "minimum": 0}})
		}
		if definition.path == "/logs" {
			for _, name := range []string{"level", "q", "run_id"} {
				parameters = append(parameters, map[string]any{"name": name, "in": "query", "schema": text})
			}
		}
		if definition.method != "get" {
			parameters = append(parameters, map[string]any{"name": "Origin", "in": "header", "description": "Required for login and session-authenticated mutations", "schema": text}, map[string]any{"name": "X-CSRF-Token", "in": "header", "description": "Required for session-authenticated mutations except login", "schema": text})
		}
		if len(parameters) > 0 {
			operation["parameters"] = parameters
		}
		if definition.path == "/events" {
			success["content"] = map[string]any{"text/event-stream": map[string]any{"schema": text}}
		}
		if definition.path == "/openapi.json" {
			success["content"] = map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object"}}}
		}
		entry, ok := paths[definition.path].(map[string]any)
		if !ok {
			entry = map[string]any{}
			paths[definition.path] = entry
		}
		entry[definition.method] = operation
	}
	return map[string]any{"openapi": "3.1.0", "info": map[string]string{"title": "ModuDock Go API", "version": s.options.Version}, "servers": []map[string]string{{"url": "/api/v1"}}, "paths": paths, "security": []map[string][]string{{"session": {}}, {"bearer": {}}}, "components": map[string]any{"schemas": schemas, "securitySchemes": map[string]any{"session": map[string]any{"type": "apiKey", "in": "cookie", "name": cookieName}, "bearer": map[string]any{"type": "http", "scheme": "bearer"}}}}
}

func securityAPISchemas(schemas map[string]any) []apiOperation {
	text := map[string]any{"type": "string"}
	boolean := map[string]any{"type": "boolean"}
	date := map[string]any{"type": "string", "format": "date-time"}
	secret := map[string]any{"type": "string", "description": "Sensitive one-time value; never persist in browser storage"}
	schemas["AuthOptions"] = objectSchema(map[string]any{"passkey_available": boolean, "passkey_reason": text}, "passkey_available", "passkey_reason")
	schemas["MFAPending"] = objectSchema(map[string]any{"mfa_required": map[string]any{"const": true}, "expires_at": date}, "mfa_required", "expires_at")
	schemas["MFACode"] = objectSchema(map[string]any{"method": map[string]any{"type": "string", "enum": []string{"totp", "recovery"}}, "code": secret}, "method", "code")
	schemas["ReauthRequest"] = objectSchema(map[string]any{"purpose": text, "password": secret, "method": text, "code": secret}, "purpose", "password")
	schemas["ReauthProof"] = objectSchema(map[string]any{"proof": secret, "expires_at": date}, "proof", "expires_at")
	schemas["ProofRequest"] = objectSchema(map[string]any{"proof": secret}, "proof")
	schemas["TOTPSetup"] = objectSchema(map[string]any{"secret": secret, "qr_png": secret, "expires_at": date}, "secret", "qr_png", "expires_at")
	schemas["TOTPConfirm"] = objectSchema(map[string]any{"proof": secret, "code": secret}, "proof", "code")
	schemas["RecoveryCodes"] = objectSchema(map[string]any{"recovery_codes": arraySchema(secret), "signed_out": map[string]any{"const": true}}, "recovery_codes", "signed_out")
	schemas["AccountSecurity"] = objectSchema(map[string]any{"totp_enabled": boolean, "recovery_codes_remaining": map[string]any{"type": "integer"}, "passkeys": arraySchema(map[string]any{"type": "object"}), "passkey_available": boolean, "passkey_reason": text}, "totp_enabled", "recovery_codes_remaining", "passkeys", "passkey_available", "passkey_reason")
	return []apiOperation{
		{"/auth/options", "get", "", "AuthOptions", "200", "", ""}, {"/auth/mfa", "post", "MFACode", "Auth", "200", "", ""}, {"/auth/login/cancel", "post", "", "", "204", "", ""},
		{"/auth/security", "get", "", "AccountSecurity", "200", "viewer", "session-only"}, {"/auth/reauth", "post", "ReauthRequest", "ReauthProof", "200", "viewer", "session-only"},
		{"/auth/totp/setup", "post", "ProofRequest", "TOTPSetup", "200", "viewer", "session-only"}, {"/auth/totp/setup", "delete", "", "", "204", "viewer", "session-only"},
		{"/auth/totp/confirm", "post", "TOTPConfirm", "RecoveryCodes", "200", "viewer", "session-only"}, {"/auth/totp", "delete", "ProofRequest", "", "204", "viewer", "session-only"},
		{"/auth/recovery-codes", "post", "ProofRequest", "RecoveryCodes", "200", "viewer", "session-only"},
	}
}

func pluginAPISchemas(schemas map[string]any) []apiOperation {
	text := map[string]any{"type": "string"}
	integer := map[string]any{"type": "integer", "minimum": 0}
	boolean := map[string]any{"type": "boolean"}
	date := map[string]any{"type": "string", "format": "date-time"}
	object := map[string]any{"type": "object", "additionalProperties": true}
	artifact := objectSchema(map[string]any{"url": map[string]any{"type": "string", "format": "uri"}, "sha256": map[string]any{"type": "string", "pattern": "^[a-fA-F0-9]{64}$"}, "executable": text}, "url", "sha256", "executable")
	task := objectSchema(map[string]any{"id": text, "title": text, "input_schema": object, "timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 3600}, "idempotent": boolean}, "id", "title", "input_schema", "timeout_seconds", "idempotent")
	resource := objectSchema(map[string]any{"id": text, "methods": arraySchema(map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "DELETE"}}), "input_schema": object, "timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 60}}, "id", "methods", "input_schema", "timeout_seconds")
	menuDeclaration := objectSchema(map[string]any{"id": text, "title": text, "icon": text, "resource": text}, "id", "title", "icon")
	schemas["Manifest"] = objectSchema(map[string]any{"schema_version": map[string]any{"const": 1}, "protocol_version": map[string]any{"const": 1}, "core_api": map[string]any{"const": 1}, "id": map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9_-]{0,63}$"}, "name": text, "version": text, "data_schema_version": map[string]any{"type": "integer", "minimum": 1}, "artifacts": map[string]any{"type": "object", "additionalProperties": artifact}, "tasks": arraySchema(task), "resources": arraySchema(resource), "menus": arraySchema(menuDeclaration), "settings_schema": object}, "schema_version", "protocol_version", "core_api", "id", "name", "version", "data_schema_version", "artifacts")
	schemas["Plugin"] = objectSchema(map[string]any{"id": text, "name": text, "version": text, "state": map[string]any{"type": "string", "enum": []string{"active", "disabled"}}, "repository_url": text, "active_release_id": text, "manifest": schemaRef("Manifest")}, "id", "name", "version", "state", "repository_url", "active_release_id", "manifest")
	schemas["Release"] = objectSchema(map[string]any{"id": text, "version": text, "commit_sha": text, "data_schema_version": map[string]any{"type": "integer", "minimum": 1}, "created_at": date}, "id", "version", "commit_sha", "data_schema_version", "created_at")
	schemas["Menu"] = objectSchema(map[string]any{"plugin_id": text, "plugin_name": text, "id": text, "title": text, "icon": text, "position": integer, "enabled": boolean}, "plugin_id", "plugin_name", "id", "title", "icon", "position", "enabled")
	schemas["Run"] = objectSchema(map[string]any{"id": text, "plugin_id": text, "release_id": text, "task_id": text, "state": map[string]any{"type": "string", "enum": []string{"queued", "running", "succeeded", "failed", "cancelled", "interrupted"}}, "progress": map[string]any{"type": "integer", "minimum": 0, "maximum": 100}, "created_at": date, "started_at": text, "finished_at": text, "error": text, "input": object, "result": object}, "id", "plugin_id", "release_id", "task_id", "state", "progress", "created_at", "started_at", "finished_at", "error")
	scheduleFields := map[string]any{"name": text, "plugin_id": text, "task_id": text, "cron": map[string]any{"type": "string", "description": "Five fields: minute hour day-of-month month day-of-week"}, "timezone": map[string]any{"type": "string", "description": "IANA timezone, e.g. Asia/Seoul or UTC"}, "input": object, "enabled": boolean}
	schemas["ScheduleWrite"] = objectSchema(scheduleFields, "name", "plugin_id", "task_id", "cron", "timezone", "input", "enabled")
	readFields := map[string]any{}
	for key, value := range scheduleFields {
		readFields[key] = value
	}
	readFields["id"] = text
	readFields["next_run_at"] = date
	readFields["last_run_at"] = text
	schemas["Schedule"] = objectSchema(readFields, "id", "name", "plugin_id", "task_id", "cron", "timezone", "input", "enabled", "next_run_at", "last_run_at")
	for _, name := range []string{"Plugin", "Release", "Menu", "Run", "Schedule"} {
		schemas[name+"List"] = objectSchema(map[string]any{"items": arraySchema(schemaRef(name)), "total": integer}, "items", "total")
	}
	schemas["PluginInstall"] = objectSchema(map[string]any{"repository_url": text, "ref": text}, "repository_url", "ref")
	schemas["PluginUpdate"] = objectSchema(map[string]any{"ref": text}, "ref")
	schemas["PluginActivate"] = objectSchema(map[string]any{"release_id": text}, "release_id")
	schemas["PluginState"] = objectSchema(map[string]any{"enabled": boolean}, "enabled")
	schemas["Settings"] = object
	schemas["ResourceData"] = object
	schemas["RunCreate"] = objectSchema(map[string]any{"input": object}, "input")
	schemas["MenuUpdate"] = objectSchema(map[string]any{"title": text, "position": integer, "enabled": boolean}, "title", "position", "enabled")
	operations := []apiOperation{
		{"/plugins", "get", "", "PluginList", "200", "viewer", "plugins:read"}, {"/plugins/{id}", "get", "", "Plugin", "200", "viewer", "plugins:read"}, {"/plugins/install", "post", "PluginInstall", "Plugin", "201", "admin", "plugins:write"}, {"/plugins/{id}/update", "post", "PluginUpdate", "Plugin", "200", "admin", "plugins:write"}, {"/plugins/{id}/releases", "get", "", "ReleaseList", "200", "admin", "plugins:read"}, {"/plugins/{id}/activate", "post", "PluginActivate", "Plugin", "200", "admin", "plugins:write"}, {"/plugins/{id}/state", "put", "PluginState", "Plugin", "200", "admin", "plugins:write"}, {"/plugins/{id}/settings", "get", "", "Settings", "200", "admin", "plugins:read"}, {"/plugins/{id}/settings", "put", "Settings", "Settings", "200", "admin", "plugins:write"},
		{"/menus", "get", "", "MenuList", "200", "viewer", "plugins:read"}, {"/menus/{plugin_id}/{menu_id}", "put", "MenuUpdate", "Menu", "200", "admin", "plugins:write"}, {"/runs", "get", "", "RunList", "200", "viewer", "runs:read"}, {"/runs/{id}", "get", "", "Run", "200", "viewer", "runs:read"}, {"/plugins/{id}/tasks/{task_id}/runs", "post", "RunCreate", "Run", "201", "operator", "runs:write"}, {"/runs/{id}/cancel", "post", "", "Run", "200", "operator", "runs:write"}, {"/schedules", "get", "", "ScheduleList", "200", "operator", "schedules:read"}, {"/schedules", "post", "ScheduleWrite", "Schedule", "201", "operator", "schedules:write"}, {"/schedules/{id}", "put", "ScheduleWrite", "Schedule", "200", "operator", "schedules:write"}, {"/schedules/{id}", "delete", "", "", "204", "operator", "schedules:write"},
	}
	for _, method := range []string{"get", "post", "put", "delete"} {
		role, scope, body := "operator", "plugins:write", "ResourceData"
		if method == "get" {
			role, scope, body = "viewer", "plugins:read", ""
		}
		operations = append(operations, apiOperation{"/plugins/{id}/resources/{resource}", method, body, "ResourceData", "200", role, scope})
	}
	return operations
}
