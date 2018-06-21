package sftp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pufferpanel/sftp"
	utils "github.com/pufferpanel/apufferi/common"
	"github.com/pufferpanel/apufferi/logging"
)

type requestPrefix struct {
	prefix string
}

func CreateRequestPrefix(prefix string) sftp.Handlers {
	h := requestPrefix{prefix: prefix}

	return sftp.Handlers{h, h, h, h, h}
}

func (rp requestPrefix) Fileread(request *sftp.Request) (io.ReaderAt, error) {
	logging.Devel("-----------------")
	logging.Devel("read request: " + request.Filepath)
	logging.Develf("Flags: %v", request.Flags)
	logging.Develf("Attributes: %v", request.Attrs)
	logging.Develf("Target: %v", request.Target)
	logging.Devel("-----------------")
	file, err := rp.getFile(request.Filepath, os.O_RDONLY, 0644)
	if err != nil {
		logging.Devel("pp-sftp internal error: ", err)
	}
	return file, err
}

func (rp requestPrefix) Filewrite(request *sftp.Request) (io.WriterAt, error) {
	logging.Devel("-----------------")
	logging.Devel("write request: " + request.Filepath)
	logging.Develf("Flags: %v", request.Flags)
	logging.Develf("Attributes: %v", request.Attrs)
	logging.Develf("Target: %v", request.Target)
	logging.Devel("-----------------")
	file, err := rp.getFile(request.Filepath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	return file, err
}

func (rp requestPrefix) Filecmd(request *sftp.Request) error {
	logging.Devel("-----------------")
	logging.Develf("cmd request [%s]: %s", request.Method, request.Filepath)
	logging.Develf("Flags: %v", request.Flags)
	logging.Develf("Attributes: %v", request.Attrs)
	logging.Develf("Target: %v", request.Target)
	logging.Devel("-----------------")
	sourceName, err := rp.validate(request.Filepath)
	if err != nil {
		logging.Devel("pp-sftp internal error: ", err)
		return rp.maskError(err)
	}
	var targetName string
	if request.Target != "" {
		targetName, err = rp.validate(request.Target)
		if err != nil {
			logging.Devel("pp-sftp internal error: ", err)
			return rp.maskError(err)
		}
	}
	switch request.Method {
	case "SetStat", "Setstat":
		{
			return nil
		}
	case "Rename":
		{
			return os.Rename(sourceName, targetName)
		}
	case "Rmdir":
		{
			return os.RemoveAll(sourceName)
		}
	case "Mkdir":
		{
			return os.Mkdir(sourceName, 0755)
		}
	case "Symlink":
		{
			return nil
		}
	case "Remove":
		{
			return os.Remove(sourceName)
		}
	default:
		return errors.New(fmt.Sprintf("Unknown request method: %v", request.Method))
	}
}

func (rp requestPrefix) Filelist(request *sftp.Request) (sftp.ListerAt, error) {
	logging.Devel("-----------------")
	logging.Develf("list request [%s]: %s", request.Method, request.Filepath)
	logging.Develf("Flags: %v", request.Flags)
	logging.Develf("Attributes: %v", request.Attrs)
	logging.Develf("Target: %v", request.Target)
	logging.Devel("-----------------")
	sourceName, err := rp.validate(request.Filepath)
	if err != nil {
		logging.Devel("pp-sftp internal error: ", err)
		return nil, rp.maskError(err)
	}
	switch request.Method {
	case "List":
		{
			file, err := os.Open(sourceName)
			if err != nil {
				return nil, rp.maskError(err)
			}
			files, err := file.Readdir(0)
			if err != nil {
				return nil, err
			}
			err = file.Close()
			if err != nil {
				return nil, rp.maskError(err)
			}
			return listerat(files), nil
		}
	case "Stat":
		{
			file, err := os.Open(sourceName)
			if err != nil {
				return nil, rp.maskError(err)
			}
			fi, err := file.Stat()
			if err != nil {
				return nil, rp.maskError(err)
			}
			err = file.Close()
			if err != nil {
				return nil, rp.maskError(err)
			}
			return listerat([]os.FileInfo{fi}), nil
		}
	case "Readlink":
		{
			target, err := os.Readlink(sourceName)
			if err != nil {
				return nil, rp.maskError(err)
			}
			file, err := os.Open(target)
			if err != nil {
				return nil, rp.maskError(err)
			}
			fi, err := file.Stat()
			if err != nil {
				return nil, rp.maskError(err)
			}
			err = file.Close()
			if err != nil {
				return nil, rp.maskError(err)
			}
			return listerat([]os.FileInfo{fi}), nil
		}
	default:
		return nil, errors.New(fmt.Sprintf("Unknown request method: %s", request.Method))
	}
}

func (rp requestPrefix) Folderopen(request *sftp.Request) error {
	logging.Devel("-----------------")
	logging.Develf("opendir request [%s]: %s", request.Method, request.Filepath)
	logging.Develf("Flags: %v", request.Flags)
	logging.Develf("Attributes: %v", request.Attrs)
	logging.Develf("Target: %v", request.Target)
	logging.Devel("-----------------")
	sourceName, err := rp.validate(request.Filepath)
	if err != nil {
		logging.Devel("pp-sftp internal error: ", err)
		return rp.maskError(err)
	}

	fi, err := os.Stat(sourceName)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return nil
	} else {
		return errors.New("not a directory")
	}
}

func (rp requestPrefix) getFile(path string, flags int, mode os.FileMode) (*os.File, error) {
	logging.Develf("Requesting path: %s", path)
	filePath, err := rp.validate(path)
	if err != nil {
		logging.Devel("pp-sftp internal error: ", err)
		return nil, rp.maskError(err)
	}

	folderPath := filepath.Dir(filePath)

	var file *os.File

	if flags&os.O_CREATE != 0 {
		_, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			err = nil
			os.MkdirAll(folderPath, 0755)
			file, err = os.Create(filePath)
		} else if err == nil {
			file, err = os.OpenFile(filePath, flags, mode)
		}
	} else {
		file, err = os.OpenFile(filePath, flags, mode)
	}
	if err != nil {
		logging.Devel("pp-sftp internal error: ", err)
		return nil, rp.maskError(err)
	}

	if file == nil {
		logging.Devel("no file loaded at this stage")
	}
	logging.Devel("no error detected on getFile")
	return file, err
}

func (rp requestPrefix) validate(path string) (string, error) {
	ok, path := rp.tryPrefix(path)
	if !ok {
		return "", errors.New("access denied")
	}
	return path, nil
}

func (rp requestPrefix) tryPrefix(path string) (bool, string) {
	newPath := filepath.Clean(filepath.Join(rp.prefix, path))
	if utils.EnsureAccess(newPath, rp.prefix) {
		return true, newPath
	} else {
		return false, ""
	}
}

func (rp requestPrefix) stripPrefix(path string) string {
	prefix, err := filepath.Abs(rp.prefix)
	if err != nil {
		prefix = rp.prefix
	}
	newStr := strings.TrimPrefix(path, prefix)
	if len(newStr) == 0 {
		newStr = "/"
	}
	return newStr
}

func (rp requestPrefix) maskError(err error) error {
	return errors.New(rp.stripPrefix(err.Error()))
}

type listerat []os.FileInfo

// Modeled after strings.Reader's ReadAt() implementation
func (f listerat) ListAt(ls []os.FileInfo, offset int64) (int, error) {
	var n int
	if offset >= int64(len(f)) {
		return 0, io.EOF
	}
	n = copy(ls, f[offset:])
	if n < len(ls) {
		return n, io.EOF
	}
	return n, nil
}