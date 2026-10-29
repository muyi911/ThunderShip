package core

import (
	"context"
	"fmt"
	"path"
)

// Extract only regular files and directories into a new, private staging directory.
const extractArchiveScript = `import sys, os, shutil, pathlib, tarfile, zipfile, stat
source, destination = sys.argv[1:]
root = pathlib.Path(destination)
root.mkdir(mode=0o700)
def unpack(name, directory, mode, stream):
    p = pathlib.PurePosixPath(name)
    if p.is_absolute() or '..' in p.parts or '\\' in name:
        raise ValueError('unsafe archive path: ' + name)
    if not p.parts:
        if directory: return
        raise ValueError('empty archive path')
    target = root.joinpath(*p.parts)
    if directory:
        target.mkdir(parents=True, exist_ok=True)
    else:
        target.parent.mkdir(parents=True, exist_ok=True)
        with target.open('xb') as out:
            shutil.copyfileobj(stream, out)
        os.chmod(target, mode & 0o777 or 0o644)
if zipfile.is_zipfile(source):
    with zipfile.ZipFile(source) as archive:
        for item in archive.infolist():
            mode = item.external_attr >> 16
            if stat.S_ISLNK(mode): raise ValueError('archive symlinks are unsupported')
            if item.is_dir(): unpack(item.filename, True, mode, None)
            else:
                with archive.open(item) as stream: unpack(item.filename, False, mode, stream)
else:
    with tarfile.open(source, 'r:*') as archive:
        for item in archive:
            if item.isdir(): unpack(item.name, True, item.mode, None)
            elif item.isfile():
                with archive.extractfile(item) as stream: unpack(item.name, False, item.mode, stream)
            else: raise ValueError('archive links and special files are unsupported')
`

func (r *Remote) archive(ctx context.Context, op RemoteFileOperation, sudoUser string) error {
	stage := path.Join(path.Dir(op.Target), ".thundership-archive-"+ID())
	if err := r.SFTP.Mkdir(stage); err != nil {
		return err
	}
	defer func() {
		if ctx.Err() == nil {
			_ = r.fileCommand(ctx, "rm -rf -- "+Quote(stage), sudoUser)
		}
	}()
	if err := r.SFTP.Chmod(stage, 0700); err != nil {
		return err
	}
	prepared := path.Join(stage, "item")
	command := "tar -czf " + Quote(prepared) + " -C " + Quote(path.Dir(op.Source)) + " -- " + Quote(path.Base(op.Source))
	if op.Action == "extract" {
		command = "python3 -c " + Quote(extractArchiveScript) + " " + Quote(op.Source) + " " + Quote(prepared)
	}
	if err := r.fileCommand(ctx, command, sudoUser); err != nil {
		return fmt.Errorf("%s失败: %w", op.Action, err)
	}
	return r.SFTP.Rename(prepared, op.Target)
}
