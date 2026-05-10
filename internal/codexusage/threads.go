package codexusage

func loadThreads(statePath string) (map[string]threadInfo, bool, error) {
	conn, err := openReadonlySQLite(statePath)
	if err != nil {
		if isOptionalCodexDBError(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer conn.Close()
	hasThreads, err := hasTable(conn, "threads")
	if err != nil {
		if isOptionalCodexDBError(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !hasThreads {
		return nil, false, nil
	}

	rows, err := conn.Query("SELECT id, COALESCE(model, ''), cwd FROM threads")
	if err != nil {
		if isOptionalCodexDBError(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer rows.Close()

	threads := make(map[string]threadInfo)
	for rows.Next() {
		var id, modelName, cwd string
		if err := rows.Scan(&id, &modelName, &cwd); err != nil {
			if isOptionalCodexDBError(err) {
				return nil, false, nil
			}
			return nil, false, err
		}
		threads[id] = threadInfo{model: modelName, cwd: cwd}
	}
	if err := rows.Err(); err != nil {
		if isOptionalCodexDBError(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return threads, true, nil
}
