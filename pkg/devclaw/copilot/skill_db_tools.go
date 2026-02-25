// Package copilot – skill_db_tools.go registers a unified tool that allows the agent
// to use the skill database for storing and retrieving structured data.
package copilot

import (
	"context"
	"fmt"
)

// RegisterSkillDBTools registers the unified skill database tool in the executor.
// This tool uses an "action" parameter to determine which operation to perform,
// reducing the number of tools from 8 separate tools to 1 unified tool.
func RegisterSkillDBTools(executor *ToolExecutor, skillDB *SkillDB) {
	if skillDB == nil {
		return
	}

	// skill_db - unified database tool for skills
	executor.Register(
		MakeToolDefinition("skill_db", "Database operations for skills to store structured data. Use action='query' to LIST data, action='insert' to ADD data.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "Operation to perform: 'query' (list records), 'insert' (add record), 'update' (modify record), 'delete' (remove record), 'create_table' (new table), 'list_tables' (show tables), 'describe' (table structure), 'drop_table' (remove table)",
					"enum":        []string{"query", "insert", "update", "delete", "create_table", "list_tables", "describe", "drop_table"},
				},
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill (lowercase, numbers, underscores only). Use underscore version: 'oss_ideas' not 'oss-ideas'",
				},
				"table_name": map[string]any{
					"type":        "string",
					"description": "Name of the table (lowercase, numbers, underscores only)",
				},
				"data": map[string]any{
					"type":        "object",
					"description": "Record data for insert/update. Keys must match column names.",
					"additionalProperties": map[string]any{},
				},
				"row_id": map[string]any{
					"type":        "string",
					"description": "ID of the record to update or delete",
				},
				"where": map[string]any{
					"type":        "object",
					"description": "Filter conditions for query (AND logic). Example: {\"status\": \"active\"}",
					"additionalProperties": map[string]any{},
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum records to return (default: 100, max: 1000)",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Records to skip for pagination",
				},
				"order_by": map[string]any{
					"type":        "string",
					"description": "Column to sort by (default: created_at)",
				},
				"order": map[string]any{
					"type":        "string",
					"description": "Sort direction: 'ASC' or 'DESC' (default: DESC)",
					"enum":        []string{"ASC", "DESC"},
				},
				"display_name": map[string]any{
					"type":        "string",
					"description": "Human-readable table name (for create_table)",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Table description (for create_table)",
				},
				"columns": map[string]any{
					"type":        "object",
					"description": "Column definitions for create_table. Keys are column names, values are SQL types like 'TEXT NOT NULL', 'INTEGER'",
					"additionalProperties": map[string]any{
						"type": "string",
					},
				},
			},
			"required": []string{"action"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			action, _ := args["action"].(string)
			if action == "" {
				return nil, fmt.Errorf("action is required")
			}

			switch action {
			case "query":
				return handleSkillDBQuery(skillDB, args)
			case "insert":
				return handleSkillDBInsert(skillDB, args)
			case "update":
				return handleSkillDBUpdate(skillDB, args)
			case "delete":
				return handleSkillDBDelete(skillDB, args)
			case "create_table":
				return handleSkillDBCreateTable(skillDB, args)
			case "list_tables":
				return handleSkillDBListTables(skillDB, args)
			case "describe":
				return handleSkillDBDescribe(skillDB, args)
			case "drop_table":
				return handleSkillDBDropTable(skillDB, args)
			default:
				return nil, fmt.Errorf("unknown action: %s", action)
			}
		},
	)
}

// handleSkillDBQuery handles the query action
func handleSkillDBQuery(skillDB *SkillDB, args map[string]any) (any, error) {
	skillName, _ := args["skill_name"].(string)
	tableName, _ := args["table_name"].(string)

	if skillName == "" || tableName == "" {
		return nil, fmt.Errorf("skill_name and table_name are required for query")
	}

	opts := QueryOptions{}
	if f, ok := args["where"].(map[string]any); ok {
		opts.Where = f
	}
	if l, ok := args["limit"].(float64); ok {
		opts.Limit = int(l)
	}
	if o, ok := args["offset"].(float64); ok {
		opts.Offset = int(o)
	}
	if ob, ok := args["order_by"].(string); ok {
		opts.OrderBy = ob
	}
	if od, ok := args["order"].(string); ok {
		opts.Order = od
	}

	results, err := skillDB.QueryWithOptions(skillName, tableName, opts)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"count":   len(results),
		"records": results,
	}, nil
}

// handleSkillDBInsert handles the insert action
func handleSkillDBInsert(skillDB *SkillDB, args map[string]any) (any, error) {
	skillName, _ := args["skill_name"].(string)
	tableName, _ := args["table_name"].(string)

	if skillName == "" || tableName == "" {
		return nil, fmt.Errorf("skill_name and table_name are required for insert")
	}

	data, ok := args["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("data must be an object")
	}

	id, err := skillDB.Insert(skillName, tableName, data)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"id":      id,
		"message": fmt.Sprintf("Record inserted with ID %s", id),
	}, nil
}

// handleSkillDBUpdate handles the update action
func handleSkillDBUpdate(skillDB *SkillDB, args map[string]any) (any, error) {
	skillName, _ := args["skill_name"].(string)
	tableName, _ := args["table_name"].(string)
	rowID, _ := args["row_id"].(string)

	if skillName == "" || tableName == "" || rowID == "" {
		return nil, fmt.Errorf("skill_name, table_name, and row_id are required for update")
	}

	data, ok := args["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("data must be an object")
	}

	err := skillDB.Update(skillName, tableName, rowID, data)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"success": true,
		"message": fmt.Sprintf("Record %s updated successfully", rowID),
	}, nil
}

// handleSkillDBDelete handles the delete action
func handleSkillDBDelete(skillDB *SkillDB, args map[string]any) (any, error) {
	skillName, _ := args["skill_name"].(string)
	tableName, _ := args["table_name"].(string)
	rowID, _ := args["row_id"].(string)

	if skillName == "" || tableName == "" || rowID == "" {
		return nil, fmt.Errorf("skill_name, table_name, and row_id are required for delete")
	}

	err := skillDB.Delete(skillName, tableName, rowID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"success": true,
		"message": fmt.Sprintf("Record %s deleted successfully", rowID),
	}, nil
}

// handleSkillDBCreateTable handles the create_table action
func handleSkillDBCreateTable(skillDB *SkillDB, args map[string]any) (any, error) {
	skillName, _ := args["skill_name"].(string)
	tableName, _ := args["table_name"].(string)
	displayName, _ := args["display_name"].(string)
	description, _ := args["description"].(string)

	if skillName == "" || tableName == "" {
		return nil, fmt.Errorf("skill_name and table_name are required for create_table")
	}

	columnsRaw, ok := args["columns"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("columns must be an object")
	}

	columns := make(map[string]string)
	for k, v := range columnsRaw {
		vs, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("column %q type must be a string", k)
		}
		columns[k] = vs
	}

	err := skillDB.CreateTable(skillName, tableName, displayName, description, columns)
	if err != nil {
		return nil, err
	}

	return fmt.Sprintf("Table '%s' created successfully for skill '%s'. Full table name: %s_%s",
		tableName, skillName, skillName, tableName), nil
}

// handleSkillDBListTables handles the list_tables action
func handleSkillDBListTables(skillDB *SkillDB, args map[string]any) (any, error) {
	skillName, _ := args["skill_name"].(string)

	tables, err := skillDB.ListTables(skillName)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"count":  len(tables),
		"tables": tables,
	}, nil
}

// handleSkillDBDescribe handles the describe action
func handleSkillDBDescribe(skillDB *SkillDB, args map[string]any) (any, error) {
	skillName, _ := args["skill_name"].(string)
	tableName, _ := args["table_name"].(string)

	if skillName == "" || tableName == "" {
		return nil, fmt.Errorf("skill_name and table_name are required for describe")
	}

	info, err := skillDB.DescribeTable(skillName, tableName)
	if err != nil {
		return nil, err
	}

	return info, nil
}

// handleSkillDBDropTable handles the drop_table action
func handleSkillDBDropTable(skillDB *SkillDB, args map[string]any) (any, error) {
	skillName, _ := args["skill_name"].(string)
	tableName, _ := args["table_name"].(string)

	if skillName == "" || tableName == "" {
		return nil, fmt.Errorf("skill_name and table_name are required for drop_table")
	}

	err := skillDB.DropTable(skillName, tableName)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"success": true,
		"message": fmt.Sprintf("Table %s_%s dropped successfully", skillName, tableName),
	}, nil
}
