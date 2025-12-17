package services

import "fmt"

type UserService struct {
	// Dependencies can be added here
}

func NewUserService() *UserService {
	return &UserService{}
}

func (s *UserService) GetWelcomeMessage(name string) string {
	return fmt.Sprintf("Welcome, %s! This message comes from the UserService.", name)
}
