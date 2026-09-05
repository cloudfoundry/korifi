// Package broker is a multi-offering Open Service Broker.
//
// Each marketplace service is an Offering (see postgres.go). Register more
// offerings in newDefaultOfferings. Instance metadata lives in a shared store
// (memory, or SQL when POSTGRES_HOST is set).
package broker
