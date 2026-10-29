package core

import "github.com/danieljoos/wincred"

func SetSecret(id, value string) error {
	c := wincred.NewGenericCredential("ShellWorkflow/" + id)
	c.CredentialBlob = []byte(value)
	c.Persist = wincred.PersistLocalMachine
	return c.Write()
}
func GetSecret(id string) (string, error) {
	c, e := wincred.GetGenericCredential("ShellWorkflow/" + id)
	if e != nil {
		return "", e
	}
	return string(c.CredentialBlob), nil
}

func DeleteSecret(id string) error {
	c, err := wincred.GetGenericCredential("ShellWorkflow/" + id)
	if err != nil {
		return nil
	}
	return c.Delete()
}
