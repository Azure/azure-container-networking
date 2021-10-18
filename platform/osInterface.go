package platform

type execClient struct{}

//nolint:revive // ExecClient make sense
type ExecClient interface {
	ExecuteCommand(command string) (string, error)
}

func NewExecClient() ExecClient {
	return &execClient{}
}
