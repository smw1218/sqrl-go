package ssp

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"

	sqrl "github.com/RaniSputnik/sqrl-go"
)

type inmemoryStore struct {
	transactions      map[sqrl.Nut]*Transaction
	firstTransactions map[sqrl.Nut]sqrl.Nut
	tokens            map[sqrl.Nut]Token
	users             []*User

	sync.Mutex
}

func NewMemoryStore() Store {
	return &inmemoryStore{
		transactions:      map[sqrl.Nut]*Transaction{},
		firstTransactions: map[sqrl.Nut]sqrl.Nut{},
		tokens:            map[sqrl.Nut]Token{},
	}
}

func (s *inmemoryStore) GetFirstTransaction(ctx context.Context, nut sqrl.Nut) (*Transaction, error) {
	s.Lock()
	defer s.Unlock()
	firstTransactionId, exists := s.firstTransactions[nut]
	if !exists {
		return nil, nil
	}
	return s.transactions[firstTransactionId], nil
}

func (s *inmemoryStore) SaveTransaction(ctx context.Context, t *Transaction) error {
	firstTransaction, err := s.GetFirstTransaction(ctx, t.Id)
	if err != nil {
		return err
	}
	if firstTransaction == nil {
		firstTransaction = t
	}

	s.Lock()
	defer s.Unlock()

	s.transactions[t.Id] = t
	s.firstTransactions[t.Next] = firstTransaction.Id
	return nil
}

func (s *inmemoryStore) SaveIdentSuccess(ctx context.Context, nut sqrl.Nut, token Token) error {
	s.Lock()
	defer s.Unlock()
	s.tokens[nut] = token
	return nil
}

func (s *inmemoryStore) GetIdentSuccess(ctx context.Context, nut sqrl.Nut) (token Token, err error) {
	s.Lock()
	defer s.Unlock()

	return s.tokens[nut], nil
}

func (s *inmemoryStore) CreateUser(ctx context.Context, idk sqrl.Identity) (*User, error) {
	s.Lock()
	defer s.Unlock()
	newUser := &User{
		Id:  uuid(),
		Idk: idk,
	}
	s.users = append(s.users, newUser)
	return newUser, nil
}

func (s *inmemoryStore) GetUserByIdentity(ctx context.Context, idk sqrl.Identity) (*User, error) {
	s.Lock()
	defer s.Unlock()

	for _, user := range s.users {
		if user.Idk == idk {
			return user, nil
		}
	}

	return nil, nil
}

func uuid() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
