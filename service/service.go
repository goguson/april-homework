package service

import (
	"context"
	"errors"
	"net/http"

	"golang.org/x/sync/errgroup"
)

type Service struct {
	httpServer *http.Server
}

func New(httpServer *http.Server) *Service {
	return &Service{httpServer: httpServer}
}

func (s *Service) Run(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error {
		err := s.httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	group.Go(func() error {
		<-ctx.Done()
		return s.httpServer.Shutdown(context.Background())
	})

	return group.Wait()
}
