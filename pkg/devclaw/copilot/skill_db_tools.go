// Package copilot – skill_db_tools.go registers individual skill database tools
// that allow the agent to store and retrieve structured data.
package copilot

import (
	"context"
	"fmt"
)

// RegisterSkillDBTools registers the skill database tools in the executor.
// Each database operation is registered as its own tool with a focused schema.
func RegisterSkillDBTools(executor *ToolExecutor, skillDB *SkillDB) {
	if skillDB == nil {
		return
	}

	// skill_db_query - query records from a skill table
	executor.Register(
		MakeToolDefinition("skill_db_query", "Query records from a skill database table. Returns matching rows with optional filtering, sorting, and pagination.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill (lowercase, numbers, underscores only). Use underscore version: 'oss_ideas' not 'oss-ideas'",
				},
				"table_name": map[string]any{
					"type":        "string",
					"description": "Name of the table (lowercase, numbers, underscores only)",
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
			},
			"required": []string{"skill_name", "table_name"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			return handleSkillDBQuery(skillDB, args)
		},
	)

	// skill_db_insert - insert a record into a skill table
	executor.RegisterHidden(
		MakeToolDefinition("skill_db_insert", "Insert a new record into a skill database table. Returns the generated record ID.", map[string]any{
			"type": "object",
			"properties": map[string]any{
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
					"description": "Record data to insert. Keys must match column names.",
					"additionalProperties": map[string]any{},
				},
			},
			"required": []string{"skill_name", "table_name", "data"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			return handleSkillDBInsert(skillDB, args)
		},
	)

	// skill_db_update - update an existing record in a skill table
	executor.RegisterHidden(
		MakeToolDefinition("skill_db_update", "Update an existing record in a skill database table by row ID.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill (lowercase, numbers, underscores only). Use underscore version: 'oss_ideas' not 'oss-ideas'",
				},
				"table_name": map[string]any{
					"type":        "string",
					"description": "Name of the table (lowercase, numbers, underscores only)",
				},
				"row_id": map[string]any{
					"type":        "string",
					"description": "ID of the record to update",
				},
				"data": map[string]any{
					"type":        "object",
					"description": "Fields to update. Keys must match column names.",
					"additionalProperties": map[string]any{},
				},
			},
			"required": []string{"skill_name", "table_name", "row_id", "data"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			return handleSkillDBUpdate(skillDB, args)
		},
	)

	// skill_db_delete - delete a record from a skill table
	executor.RegisterHidden(
		MakeToolDefinition("skill_db_delete", "Delete a record from a skill database table by row ID.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill (lowercase, numbers, underscores only). Use underscore version: 'oss_ideas' not 'oss-ideas'",
				},
				"table_name": map[string]any{
					"type":        "string",
					"description": "Name of the table (lowercase, numbers, underscores only)",
				},
				"row_id": map[string]any{
					"type":        "string",
					"description": "ID of the record to delete",
				},
			},
			"required": []string{"skill_name", "table_name", "row_id"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			return handleSkillDBDelete(skillDB, args)
		},
	)

	// skill_db_create_table - create a new table for a skill
	executor.RegisterHidden(
		MakeToolDefinition("skill_db_create_table", "Create a new database table for a skill with custom column definitions.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill (lowercase, numbers, underscores only). Use underscore version: 'oss_ideas' not 'oss-ideas'",
				},
				"table_name": map[string]any{
					"type":        "string",
					"description": "Name of the table (lowercase, numbers, underscores only)",
				},
				"display_name": map[string]any{
					"type":        "string",
					"description": "Human-readable table name",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Table description",
				},
				"columns": map[string]any{
					"type":        "object",
					"description": "Column definitions. Keys are column names, values are SQL types like 'TEXT NOT NULL', 'INTEGER'",
					"additionalProperties": map[string]any{
						"type": "string",
					},
				},
			},
			"required": []string{"skill_name", "table_name", "columns"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			return handleSkillDBCreateTable(skillDB, args)
		},
	)

	// skill_db_list_tables - list all tables for a skill (or all skills)
	executor.Register(
		MakeToolDefinition("skill_db_list_tables", "List all database tables, optionally filtered by skill name.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill to filter by. If omitted, lists tables for all skills.",
				},
			},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			return handleSkillDBListTables(skillDB, args)
		},
	)

	// skill_db_describe - describe the structure of a skill table
	executor.RegisterHidden(
		MakeToolDefinition("skill_db_describe", "Describe the structure of a skill database table, including column names and types.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill (lowercase, numbers, underscores only). Use underscore version: 'oss_ideas' not 'oss-ideas'",
				},
				"table_name": map[string]any{
					"type":        "string",
					"description": "Name of the table (lowercase, numbers, underscores only)",
				},
			},
			"required": []string{"skill_name", "table_name"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			return handleSkillDBDescribe(skillDB, args)
		},
	)

	// skill_db_drop_table - drop (permanently delete) a skill table
	executor.RegisterHidden(
		MakeToolDefinition("skill_db_drop_table", "Permanently drop a skill database table and all its data.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Name of the skill (lowercase, numbers, underscores only). Use underscore version: 'oss_ideas' not 'oss-ideas'",
				},
				"table_name": map[string]any{
					"type":        "string",
					"description": "Name of the table (lowercase, numbers, underscores only)",
				},
			},
			"required": []string{"skill_name", "table_name"},
		}),
		func(_ context.Context, args map[string]any) (any, error) {
			return handleSkillDBDropTable(skillDB, args)
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
