package api

import "github.com/fanboykun/watcher/internal/database"

// nonNilSlice guarantees collection responses are encoded as [] instead of null.
func nonNilSlice[T any](values []T) []T {
	if values == nil {
		return make([]T, 0)
	}
	return values
}

// normalizeServices guarantees JSON array fields are encoded as [] instead of null.
func normalizeServices(services []database.Service) []database.Service {
	services = nonNilSlice(services)
	for i := range services {
		normalizeServiceCollections(&services[i])
	}
	return services
}

// normalizeServiceCollections guarantees nested service collections encode as arrays.
func normalizeServiceCollections(service *database.Service) {
	if service == nil {
		return
	}
	if service.ConfigFiles == nil {
		service.ConfigFiles = make([]database.ServiceConfigFile, 0)
	}
}

// normalizeWatcherServices applies stable collection semantics to one watcher response.
func normalizeWatcherServices(watcher *database.Watcher) {
	if watcher == nil {
		return
	}
	watcher.Services = normalizeServices(watcher.Services)
}
