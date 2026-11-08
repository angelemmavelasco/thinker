# Database

#### How can i add a new table and whats the step-by-step i must follow?
In order to add a new table, there a sequence of steps to follow:
1. `internal/database/schema.sql`: define the schema of the new table, taking care about fks, for example:
```sql
CREATE TABLE IF NOT EXISTS new_table (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    fk_id INT REFERENCES other_table(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

```
2. `internal/database/query.sql`: add necessary queries before code into go files.

```sql
-- name: createNewItem :one
INSERT INTO new_table (...)
...
RETURNING *;
```

It's very important to decalre intial statements at the begining of the
function you want to create. The basic syntax is:
-- name: <query_name> :<return_type>

<query_name> recommended format:
Action/Feature + Object-TableName.

Some examples:
- CreateItem
- GetItem
- UpdateItem
- DeleteItem
- ListItems

<return_type> types:
- :one - returns a single row
- :many - returns multiple rows
- :exec - executes a statement without returning rows

3. From the project root, execute:
```bash
sqlc generate
```
*You should have installed sqlc before running the command
you can install it by running the following command:
```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest.
```

This is gonna update internal/database/query.sql.go with the new queries and types and
internal/database/models.go.

*You must not modify by hand the generated files, if you need to change something, change the sql files and run `sqlc generate` again.


5. If the new table is gonna be used from an HTTP route,
add or update the corresponding DTO and Handler:
```bash
internal/dtos/<feature>.go
internal/handlers/<feature>.go
```

*Note: business logic must be implemented in the service layer, not in the handler.
Thats the difference between the handler and the service layer,
the handler is just a bridge between the HTTP request and the service layer,
it should not contain any business logic.
Additionally, dtos is used to structure responses or inputs, its 
different from the models, which are used to structure the database tables.
