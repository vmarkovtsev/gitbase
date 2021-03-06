package gitbase

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/src-d/go-git-fixtures.v3"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

func TestRepository(t *testing.T) {
	require := require.New(t)

	gitRepo := &git.Repository{}
	repo := NewRepository("identifier", gitRepo)

	require.Equal("identifier", repo.ID)
	require.Equal(gitRepo, repo.Repository)

	repo = NewRepository("/other/path", nil)

	require.Equal("/other/path", repo.ID)
	require.Nil(repo.Repository)
}

func TestRepositoryPoolBasic(t *testing.T) {
	require := require.New(t)

	pool := NewRepositoryPool()

	repo, err := pool.GetPos(0)
	require.Nil(repo)
	require.Equal(io.EOF, err)

	repo, err = pool.GetRepo("foo")
	require.Nil(repo)
	require.EqualError(err, ErrPoolRepoNotFound.New("foo").Error())

	pool.Add(gitRepo("0", "/directory/should/not/exist"))
	repo, err = pool.GetPos(0)
	require.Nil(repo)
	require.EqualError(err, git.ErrRepositoryNotExists.Error())

	_, err = pool.GetPos(1)
	require.Equal(io.EOF, err)

	path := fixtures.Basic().ByTag("worktree").One().Worktree().Root()

	err = pool.Add(gitRepo("1", path))
	require.NoError(err)

	repo, err = pool.GetPos(1)
	require.NoError(err)
	require.Equal("1", repo.ID)
	require.NotNil(repo)

	repo, err = pool.GetRepo("1")
	require.NoError(err)
	require.Equal("1", repo.ID)
	require.NotNil(repo)

	err = pool.Add(gitRepo("1", path))
	require.Error(err)
	require.True(errRepoAlreadyRegistered.Is(err))

	_, err = pool.GetPos(0)
	require.Equal(git.ErrRepositoryNotExists, err)

	_, err = pool.GetPos(2)
	require.Equal(io.EOF, err)
}

func TestRepositoryPoolGit(t *testing.T) {
	require := require.New(t)

	path := fixtures.Basic().ByTag("worktree").One().Worktree().Root()

	pool := NewRepositoryPool()

	require.NoError(pool.AddGit(path))

	repo, err := pool.GetPos(0)
	require.Equal(path, repo.ID)
	require.NotNil(repo)
	require.NoError(err)

	iter, err := repo.CommitObjects()
	require.NoError(err)

	count := 0

	for {
		commit, err := iter.Next()
		if err != nil {
			break
		}

		require.NotNil(commit)

		count++
	}

	require.Equal(9, count)
}

func TestRepositoryPoolIterator(t *testing.T) {
	require := require.New(t)

	path := fixtures.Basic().ByTag("worktree").One().Worktree().Root()

	pool := NewRepositoryPool()
	pool.Add(gitRepo("0", path))
	pool.Add(gitRepo("1", path))

	iter, err := pool.RepoIter()
	require.NoError(err)

	count := 0

	for {
		repo, err := iter.Next()
		if err != nil {
			require.Equal(io.EOF, err)
			break
		}

		require.NotNil(repo)
		require.Equal(strconv.Itoa(count), repo.ID)

		count++
	}

	require.Equal(2, count)
}

func TestRepositoryPoolSiva(t *testing.T) {
	require := require.New(t)

	expectedRepos := 3

	pool := NewRepositoryPool()
	path := filepath.Join(
		os.Getenv("GOPATH"),
		"src", "github.com", "src-d", "gitbase",
		"_testdata",
	)

	require.NoError(
		filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if IsSivaFile(path) {
				require.NoError(pool.AddSivaFile(path))
			}

			return nil
		}),
	)

	require.Equal(expectedRepos, len(pool.repositories))

	expected := []int{606, 452, 75}
	result := make([]int, expectedRepos)

	for i := 0; i < expectedRepos; i++ {
		repo, err := pool.GetPos(i)
		require.NoError(err)

		iter, err := repo.CommitObjects()
		require.NoError(err)

		require.NoError(iter.ForEach(func(c *object.Commit) error {
			result[i]++
			return nil
		}))
	}

	require.Equal(expected, result)
}

var errIter = fmt.Errorf("Error iter")
