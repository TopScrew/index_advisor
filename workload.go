package main

import (
	"fmt"
	"math"
	"strings"
)

type SQLType int

const (
	SQLTypeSelect SQLType = iota
	SQLTypeInsert
	SQLTypeUpdate
	SQLTypeOthers
)

type SQL struct { // DQL or DML
	Alias            string
	SchemaName       string
	Text             string
	Frequency        int
	IndexableColumns Set[Column] // Indexable columns related to this SQL
	Plans            []Plan      // A SQL may have multiple different plans
}

func (sql SQL) Type() SQLType {
	text := strings.TrimSpace(sql.Text)
	if len(text) < 6 {
		return SQLTypeOthers
	}
	prefix := strings.ToLower(text[:6])
	if strings.HasPrefix(prefix, "select") {
		return SQLTypeSelect
	}
	if strings.HasPrefix(prefix, "insert") {
		return SQLTypeInsert
	}
	if strings.HasPrefix(prefix, "update") {
		return SQLTypeUpdate
	}
	return SQLTypeOthers
}

func (sql SQL) Key() string {
	return sql.Text
}

type TableSchema struct {
	SchemaName     string
	TableName      string
	Columns        []Column
	Indexes        []Index
	CreateStmtText string // `create table t (...)`
}

func (t TableSchema) Key() string {
	return fmt.Sprintf("%v.%v", t.SchemaName, t.TableName)
}

type TableStats struct {
	SchemaName    string
	TableName     string
	StatsFilePath string
}

func (t TableStats) Key() string {
	return fmt.Sprintf("%v.%v", t.SchemaName, t.TableName)
}

type Column struct {
	SchemaName string
	TableName  string
	ColumnName string
}

func NewColumn(schemaName, tableName, columnName string) Column {
	return Column{SchemaName: strings.ToLower(schemaName), TableName: strings.ToLower(tableName), ColumnName: strings.ToLower(columnName)}
}

func NewColumns(schemaName, tableName string, columnNames ...string) []Column {
	var cols []Column
	for _, col := range columnNames {
		cols = append(cols, NewColumn(schemaName, tableName, col))
	}
	return cols
}

func (c Column) Key() string {
	return fmt.Sprintf("%v.%v.%v", c.SchemaName, c.TableName, c.ColumnName)
}

func (c Column) String() string {
	return fmt.Sprintf("%v.%v.%v", c.SchemaName, c.TableName, c.ColumnName)
}

type Index struct {
	SchemaName string
	TableName  string
	IndexName  string
	Columns    []Column
}

func NewIndex(schemaName, tableName, indexName string, columns ...string) Index {
	return Index{SchemaName: strings.ToLower(schemaName), TableName: strings.ToLower(tableName), IndexName: strings.ToLower(indexName), Columns: NewColumns(schemaName, tableName, columns...)}
}

func (i Index) columnNames() []string {
	var names []string
	for _, col := range i.Columns {
		names = append(names, col.ColumnName)
	}
	return names
}

func (i Index) DDL() string {
	return fmt.Sprintf("CREATE INDEX %v ON %v.%v (%v)", i.IndexName, i.SchemaName, i.TableName, strings.Join(i.columnNames(), ", "))
}

func (i Index) Key() string {
	return fmt.Sprintf("%v.%v(%v)", i.SchemaName, i.TableName, strings.Join(i.columnNames(), ","))
}

// PrefixContain returns whether j is a prefix of i.
func (i Index) PrefixContain(j Index) bool {
	if i.SchemaName != j.SchemaName || i.TableName != j.TableName || len(i.Columns) < len(j.Columns) {
		return false
	}
	for k := range j.Columns {
		if i.Columns[k].ColumnName != j.Columns[k].ColumnName {
			return false
		}
	}
	return true
}

type Plan struct {
}

type SampleRows struct {
	TableName string
}

type WorkloadInfo struct {
	SQLs             Set[SQL]
	TableSchemas     Set[TableSchema]
	TableStats       Set[TableStats]
	IndexableColumns Set[Column]
	SampleRows       []SampleRows
}

// AllSchemaNames returns all schema names in this workload.
func (w WorkloadInfo) AllSchemaNames() []string {
	x := make(map[string]struct{})
	result := make([]string, 0)
	for _, t := range w.TableSchemas.ToList() {
		if _, ok := x[t.SchemaName]; !ok {
			result = append(result, t.SchemaName)
			x[t.SchemaName] = struct{}{}
		}
	}
	return result
}

// IndexConfCost is the cost of a index configuration.
type IndexConfCost struct {
	TotalWorkloadQueryCost    float64
	TotalNumberOfIndexColumns int
}

func (c IndexConfCost) Less(other IndexConfCost) bool {
	if c.TotalNumberOfIndexColumns == 0 { // not initialized
		return false
	}
	if other.TotalNumberOfIndexColumns == 0 { // not initialized
		return true
	}
	cc, cOther := c.TotalWorkloadQueryCost, other.TotalWorkloadQueryCost
	if math.Abs(cc-cOther) < 10 || math.Abs(cc-cOther)/math.Max(cc, cOther) < 0.01 {
		// if they have the same cost, then the less columns, the better.
		return c.TotalNumberOfIndexColumns < other.TotalNumberOfIndexColumns
	}
	return c.TotalWorkloadQueryCost < other.TotalWorkloadQueryCost
}
