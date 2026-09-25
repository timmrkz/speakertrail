# Deploying to Scaleway

Every merge to `main` deploys itself once CI is green: GitHub Actions builds the image, pushes it to the Scaleway Container Registry and updates two things in Paris (fr-par).

| What | Scaleway product | Runs |
| --- | --- | --- |
| `speakertrail-web`, the web interface | Serverless Container, scales to zero | When someone opens it |
| `speakertrail-nightly`, the crawl | Serverless Job | Every night at 05:00 Berlin time |
| The database | Serverless SQL Database, PostgreSQL | Scales to zero when idle |

Until the secrets below are set, the deploy workflow only lists what is missing and does nothing else. So merging before the setup is harmless.

## One-time setup, about 20 minutes

You do this once in the Scaleway console. No command line needed.

### 1. Account and project

1. Create an account at console.scaleway.com and add a payment method.
2. Use the default project, or create one called `speakertrail`. Copy its **Project ID** from the project settings.
3. Copy your **Organization ID** from the organisation settings.

### 2. The database

1. Open Databases, then Serverless SQL Databases, then Create.
2. Region Paris, name `speakertrail`, minimum 0 vCPU, maximum 2 vCPU.
3. When it is ready, open it and copy the connection details. You need the host and the database name.

### 3. Two API keys

One key is for GitHub to deploy. The other is only for the app to reach its database.

1. Open IAM, then Applications, then Create application `speakertrail-deploy`.
2. Give it a policy with these permission sets, scoped to the project:
   `ContainerRegistryFullAccess`, `ContainersFullAccess`, `ServerlessJobsFullAccess`.
3. Create an API key for it. Copy the **access key** and the **secret key**. The secret key is shown only once.
4. Create a second application `speakertrail-app` with the permission set `ServerlessSQLDatabaseReadWrite`, and an API key for it. Copy its **application ID** and **secret key**.

The database address is then:

```
postgres://<speakertrail-app application ID>:<speakertrail-app secret key>@<database host>:5432/<database name>?sslmode=require
```

### 4. The login for the interface

On any machine with Go, in this repository:

```sh
go run ./cmd/speakertrail hash-password
```

Type a password of at least 12 characters. It prints a hash that starts with `$2a$`. Also make a session secret, any random string of 32 characters or more, for example with `openssl rand -hex 32`.

### 5. The GitHub secrets

In the repository on GitHub, open Settings, then Secrets and variables, then Actions, and add:

| Secret | Value |
| --- | --- |
| `SCW_ACCESS_KEY` | Access key of `speakertrail-deploy` |
| `SCW_SECRET_KEY` | Secret key of `speakertrail-deploy` |
| `SCW_DEFAULT_PROJECT_ID` | Project ID |
| `SCW_DEFAULT_ORGANIZATION_ID` | Organization ID |
| `DATABASE_URL` | The database address from step 3 |
| `UI_PASSWORD_HASH` | The hash from step 4 |
| `SESSION_SECRET` | The random string from step 4 |

### 6. First deploy

Open Actions, then Deploy, then Run workflow. After about 5 minutes the summary shows the address of the interface, ending in `functions.fnc.fr-par.scw.cloud`. Log in with your password.

The first nightly crawl runs at 05:00. To start one now, open Serverless Jobs in the console, pick `speakertrail-nightly` and choose Run job. It imports the sources from the brief on its first run.

## Worth knowing

- The job gets `DATABASE_URL` as a plain environment variable, because Serverless Jobs only take secrets from Secret Manager. Only members of the project can see it. Moving it to Secret Manager is a later step.
- To change the crawl time, edit `NIGHTLY_SCHEDULE` in `deploy.sh`. It is a cron line in Berlin time.
- Chromium needs about 1 to 2 GB. The job has 2 GB and the web container 1 GB, both well inside the free allowance at this scale.
- Scaleway keeps a daily backup of the database for 7 days. The weekly own backup with a restore test is still to come.
- A custom domain is set in the container's settings under Endpoints. Nothing in the code changes.
