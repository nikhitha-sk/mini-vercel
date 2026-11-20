# broken-build

A project whose build always fails. Use it to see how Mini Vercel reports a
failed build: the deployment goes to **Error**, stderr is highlighted in the
logs, nothing is served, and the job is **not** retried (user errors fail fast).
