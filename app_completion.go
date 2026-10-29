package main

func (a *App) CompleteTerminalPath(id, directory, word string, directoriesOnly bool) ([]string, error) {
	if err := remotePath(directory); err != nil {
		return nil, err
	}
	c, err := a.getConnection(id)
	if err != nil {
		return nil, err
	}
	return c.terminal.Remote.CompletePath(directory, word, directoriesOnly)
}
