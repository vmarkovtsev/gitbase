package gitbase

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync/atomic"

	"gopkg.in/src-d/go-billy-siva.v4"
	billy "gopkg.in/src-d/go-billy.v4"
	"gopkg.in/src-d/go-billy.v4/osfs"
	errors "gopkg.in/src-d/go-errors.v1"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/storage/filesystem"
)

var (
	errInvalidRepoKind       = errors.NewKind("the repository is not: %s")
	errRepoAlreadyRegistered = errors.NewKind("the repository is already registered: %s")
	errRepoCannotOpen        = errors.NewKind("the repository could not be opened: %s")
)

// Repository struct holds an initialized repository and its ID
type Repository struct {
	*git.Repository
	ID string
}

// NewRepository creates and initializes a new Repository structure
func NewRepository(id string, repo *git.Repository) *Repository {
	return &Repository{
		Repository: repo,
		ID:         id,
	}
}

// NewRepositoryFromPath creates and initializes a new Repository structure
// and initializes a go-git repository
func NewRepositoryFromPath(id, path string) (*Repository, error) {
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}

	return NewRepository(id, repo), nil
}

// NewSivaRepositoryFromPath creates and initializes a new Repository structure
// and initializes a go-git repository backed by a siva file.
func NewSivaRepositoryFromPath(id, path string) (*Repository, error) {
	localfs := osfs.New(filepath.Dir(path))

	tmpDir, err := ioutil.TempDir(os.TempDir(), "gitbase-siva")
	if err != nil {
		return nil, err
	}

	tmpfs := osfs.New(tmpDir)

	fs, err := sivafs.NewFilesystem(localfs, filepath.Base(path), tmpfs)
	if err != nil {
		return nil, err
	}

	sto, err := filesystem.NewStorage(fs)
	if err != nil {
		return nil, err
	}

	repo, err := git.Open(sto, nil)
	if err != nil {
		return nil, err
	}

	return NewRepository(id, repo), nil
}

type repository interface {
	ID() string
	Repo() (*Repository, error)
	FS() (billy.Filesystem, error)
	Path() string
}

type gitRepository struct {
	id   string
	path string
}

func gitRepo(id, path string) repository {
	return &gitRepository{id, path}
}

func (r *gitRepository) ID() string {
	return r.id
}

func (r *gitRepository) Repo() (*Repository, error) {
	return NewRepositoryFromPath(r.id, r.path)
}

func (r *gitRepository) FS() (billy.Filesystem, error) {
	return osfs.New(r.path), nil
}

func (r *gitRepository) Path() string {
	return r.path
}

type sivaRepository struct {
	id   string
	path string
}

func sivaRepo(id, path string) repository {
	return &sivaRepository{id, path}
}

func (r *sivaRepository) ID() string {
	return r.id
}

func (r *sivaRepository) Repo() (*Repository, error) {
	return NewSivaRepositoryFromPath(r.id, r.path)
}

func (r *sivaRepository) FS() (billy.Filesystem, error) {
	localfs := osfs.New(filepath.Dir(r.path))

	tmpDir, err := ioutil.TempDir(os.TempDir(), "gitbase-siva")
	if err != nil {
		return nil, err
	}

	tmpfs := osfs.New(tmpDir)

	return sivafs.NewFilesystem(localfs, filepath.Base(r.path), tmpfs)
}

func (r *sivaRepository) Path() string {
	return r.path
}

// RepositoryPool holds a pool git repository paths and
// functionality to open and iterate them.
type RepositoryPool struct {
	repositories map[string]repository
	idOrder      []string
}

// NewRepositoryPool initializes a new RepositoryPool
func NewRepositoryPool() *RepositoryPool {
	return &RepositoryPool{
		repositories: make(map[string]repository),
	}
}

// Add inserts a new repository in the pool.
func (p *RepositoryPool) Add(repo repository) error {
	id := repo.ID()
	if r, ok := p.repositories[id]; ok {
		return errRepoAlreadyRegistered.New(r.Path())
	}

	p.idOrder = append(p.idOrder, id)
	p.repositories[id] = repo

	return nil
}

// AddGit adds a git repository to the pool. It also sets its path as ID.
func (p *RepositoryPool) AddGit(path string) error {
	return p.AddGitWithID(path, path)
}

// AddGitWithID adds a git repository to the pool. ID should be specified.
func (p *RepositoryPool) AddGitWithID(id, path string) error {
	return p.Add(gitRepo(id, path))
}

// AddSivaFile adds a siva file to the pool. It also sets its path as ID.
func (p *RepositoryPool) AddSivaFile(path string) error {
	return p.Add(sivaRepo(path, path))
}

// AddSivaFileWithID adds a siva file to the pool. ID should be specified.
func (p *RepositoryPool) AddSivaFileWithID(id, path string) error {
	return p.Add(sivaRepo(id, path))
}

// GetPos retrieves a repository at a given position. If the position is
// out of bounds it returns io.EOF.
func (p *RepositoryPool) GetPos(pos int) (*Repository, error) {
	if pos >= len(p.repositories) {
		return nil, io.EOF
	}

	id := p.idOrder[pos]
	if id == "" {
		return nil, io.EOF
	}

	return p.GetRepo(id)
}

// ErrPoolRepoNotFound is returned when a repository id is not present in the pool.
var ErrPoolRepoNotFound = errors.NewKind("repository id %s not found in the pool")

// GetRepo returns a repository with the given id from the pool.
func (p *RepositoryPool) GetRepo(id string) (*Repository, error) {
	r, ok := p.repositories[id]
	if !ok {
		return nil, ErrPoolRepoNotFound.New(id)
	}

	return r.Repo()
}

// RepoIter creates a new Repository iterator
func (p *RepositoryPool) RepoIter() (*RepositoryIter, error) {
	iter := &RepositoryIter{
		pool: p,
	}
	atomic.StoreInt32(&iter.pos, 0)

	return iter, nil
}

// RepositoryIter iterates over all repositories in the pool
type RepositoryIter struct {
	pos  int32
	pool *RepositoryPool
}

// Next retrieves the next Repository. It returns io.EOF as error
// when there are no more Repositories to retrieve.
func (i *RepositoryIter) Next() (*Repository, error) {
	pos := int(atomic.LoadInt32(&i.pos))
	r, err := i.pool.GetPos(pos)
	atomic.AddInt32(&i.pos, 1)

	return r, err
}

// Close finished iterator. It's no-op.
func (i *RepositoryIter) Close() error {
	return nil
}
