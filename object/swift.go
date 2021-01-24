package object

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"strings"

	"github.com/ncw/swift"
)

type SwiftOSS struct {
	conn		*swift.Connection
	region		string
	storageUrl	string
	container 	string
}

func (s *SwiftOSS) String() string {
	return fmt.Sprintf("region:[%s], %s/%s", s.region, s.storageUrl, s.container)
}

func (s *SwiftOSS) Create() error {
	var err error
	err = s.conn.ContainerCreate(s.container, nil)
	if err != nil {
		fmt.Printf("ContainerCreate error: %v", err)
	}
	return err
}

func (s *SwiftOSS) Head(key string) (*Object, error) {
	info, _, err := s.conn.Object(s.container, key)
	if err != nil {
		return nil, err
	}

	mtime := info.LastModified
	size := info.Bytes

	return &Object{
		key,
		size,
		mtime,
		info.PseudoDirectory,
	}, nil
}

func (s *SwiftOSS) Get(key string, off, limit int64) (io.ReadCloser, error) {
	objOpenFile, _, err := s.conn.ObjectOpen(s.container, key, false, nil)
	if err != nil {
		fmt.Printf("ObjectOpen error: %v", err)
		return nil, err
	}
	if off > 0 {
		_, err := objOpenFile.Seek(off, 0)
		if err != nil {
			objOpenFile.Close()
			fmt.Printf("object seek error: %v", err)
			return nil, err
		}
	}
	if limit > 0 {
		defer objOpenFile.Close()
		buf := make([]byte, limit)
		if n, err := objOpenFile.Read(buf); err != nil {
			return nil, err
		} else {
			return ioutil.NopCloser(bytes.NewBuffer(buf[:n])), nil
		}
	}
	return objOpenFile, err
}

func (s *SwiftOSS) Put(key string, in io.Reader) error {
	_, err := s.conn.ObjectPut(s.container, key, in, false, "", "", nil)
	return err
}

func (s *SwiftOSS) Copy(dst, src string) error {
	return notSupported
}

func (s *SwiftOSS) Delete(key string) error {
	err := s.conn.ObjectDelete(s.container, key)
	return err
}

func (s *SwiftOSS) List(prefix, marker string, limit int64) ([]*Object, error) {
	return nil, notSupported
}

func (s *SwiftOSS) ListAll(prefix, marker string) (<-chan *Object, error) {
	return nil, notSupported
}

func (s *SwiftOSS) CreateMultipartUpload(key string) (*MultipartUpload, error) {
	return nil, notSupported
}

func (s *SwiftOSS) UploadPart(key string, uploadID string, num int, body []byte) (*Part, error) {
	return nil, notSupported
}

func (s *SwiftOSS) AbortUpload(key string, uploadID string) {
	return
}

func (s *SwiftOSS) CompleteUpload(key string, uploadID string, parts []*Part) error {
	return notSupported
}

func (s *SwiftOSS) ListUploads(marker string) ([]*PendingPart, string, error) {
	return nil, "", notSupported
}


/*
*   I use a format for swift:    http://1.2.3.4:8080/auth/v1.0#container 
*   normal access swift object storage, need two step:
*   1.  $USER access AUTH_URL(such as: http://1.2.3.4:8080/auth/v1.0"), return a $TOKEN and $StorageURL(http://1.2.3.4:8080/v1/AUTH_$USER)
*   2.  use $TOKEN to HEAD/GET/PUT/DELETE $StorageURL:
*         container: http://1.2.3.4:8080/v1/AUTH_$USER/ContainerName
*         object: http://1.2.3.4:8080/v1/AUTH_$USER/ContainerName/ObjectName
*
*    So:  I define a format endpoint:  auth_url + "#" + containerName <====>  http://1.2.3.4:8080/auth/v1.0#container
*/
func newSwiftOSS(endpoint, accessKey, secretKey string) (ObjectStorage, error) {
	uri, err := url.ParseRequestURI(endpoint)
	if err != nil {
		return nil, fmt.Errorf("Invalid endpoint %s: %s", endpoint, err)
	}
	//use 'http' or 'https"
	if uri.Scheme != "http" && uri.Scheme != "https" {
		return nil, fmt.Errorf("Invalid uri Scheme")
	}

	//current only add support for v1 authentication
	if !strings.HasPrefix(uri.Path, "/auth/v1.0#") {
		return nil, fmt.Errorf("Invalid auth method: %s", uri.Path)
	}
	temp := strings.SplitN(uri.Path, "#", 2)
	
	auth_url := uri.Scheme + "://" + uri.Host + temp[0]
	container := temp[1]
	if strings.Contains(container, "/") {
		return nil, fmt.Errorf("Invalid container name: %s", container)
	}

	//fmt.Printf("endpoint: %s\n", endpoint)
	//fmt.Printf("connect to: %s, container: %s, auth_key: %s, ApiKey: *removed*\n", auth_url, container, accessKey)
	conn := swift.Connection{
		UserName: accessKey,
		ApiKey:   secretKey,
		AuthUrl:  auth_url,
	}
	// Authenticate
	err = conn.Authenticate()
	if err != nil {
		fmt.Printf("auth failed\n")
		return nil, err
	}
	return &SwiftOSS{&conn, conn.Region, conn.StorageUrl, container}, nil
}

func init() {
	Register("swift", newSwiftOSS)
}
