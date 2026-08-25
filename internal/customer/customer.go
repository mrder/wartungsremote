// Package customer implements customer/group management per
// docs/PROJECT_CONCEPT.md §28 and docs/API.md §16.
package customer

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("customer: not found")

type Customer struct {
	ID             uuid.UUID
	Name           string
	CustomerNumber string
	Status         string
	Notes          string
}

type Group struct {
	ID         uuid.UUID
	CustomerID *uuid.UUID
	Name       string
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) List(ctx context.Context) ([]Customer, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, COALESCE(customer_number,''), status, COALESCE(notes,'') FROM customers ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("customer: list: %w", err)
	}
	defer rows.Close()
	out := []Customer{}
	for rows.Next() {
		var c Customer
		if err := rows.Scan(&c.ID, &c.Name, &c.CustomerNumber, &c.Status, &c.Notes); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repo) Create(ctx context.Context, name, number, notes string) (Customer, error) {
	c := Customer{Name: name, CustomerNumber: number, Notes: notes, Status: "active"}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO customers (name, customer_number, notes, status) VALUES ($1,$2,$3,'active') RETURNING id
	`, name, number, notes).Scan(&c.ID)
	if err != nil {
		return Customer{}, fmt.Errorf("customer: create: %w", err)
	}
	return c, nil
}

func (r *Repo) Update(ctx context.Context, id uuid.UUID, name, number, notes, status string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE customers SET name=$2, customer_number=$3, notes=$4, status=$5, updated_at=now() WHERE id=$1
	`, id, name, number, notes, status)
	if err != nil {
		return fmt.Errorf("customer: update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) Get(ctx context.Context, id uuid.UUID) (Customer, error) {
	var c Customer
	err := r.pool.QueryRow(ctx, `SELECT id, name, COALESCE(customer_number,''), status, COALESCE(notes,'') FROM customers WHERE id=$1`, id).
		Scan(&c.ID, &c.Name, &c.CustomerNumber, &c.Status, &c.Notes)
	if errors.Is(err, pgx.ErrNoRows) {
		return Customer{}, ErrNotFound
	}
	if err != nil {
		return Customer{}, fmt.Errorf("customer: get: %w", err)
	}
	return c, nil
}

func (r *Repo) ListGroups(ctx context.Context, customerID *uuid.UUID) ([]Group, error) {
	var rows pgx.Rows
	var err error
	if customerID != nil {
		rows, err = r.pool.Query(ctx, `SELECT id, customer_id, name FROM device_groups WHERE customer_id = $1 ORDER BY name`, *customerID)
	} else {
		rows, err = r.pool.Query(ctx, `SELECT id, customer_id, name FROM device_groups ORDER BY name`)
	}
	if err != nil {
		return nil, fmt.Errorf("customer: list groups: %w", err)
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.CustomerID, &g.Name); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *Repo) CreateGroup(ctx context.Context, customerID *uuid.UUID, name string) (Group, error) {
	g := Group{CustomerID: customerID, Name: name}
	err := r.pool.QueryRow(ctx, `INSERT INTO device_groups (customer_id, name) VALUES ($1,$2) RETURNING id`, customerID, name).Scan(&g.ID)
	if err != nil {
		return Group{}, fmt.Errorf("customer: create group: %w", err)
	}
	return g, nil
}
