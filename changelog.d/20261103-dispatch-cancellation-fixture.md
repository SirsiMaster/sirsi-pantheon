Make the dispatch cancellation regression fixture wait for its canceled caller before ending the HTTP response, avoiding an empty-response race in CI.
