package domain

//go:generate mockgen -destination=mocks/mock_publisher.go -package=mocks . AvatarEventPublisher

import "context"

type AvatarEventPublisher interface {
	PublishUploadEvent(ctx context.Context, event AvatarUploadEvent) error
	PublishDeleteEvent(ctx context.Context, event AvatarDeleteEvent) error
}
