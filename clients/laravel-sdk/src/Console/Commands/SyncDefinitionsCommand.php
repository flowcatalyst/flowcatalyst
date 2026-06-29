<?php

declare(strict_types=1);

namespace FlowCatalyst\Console\Commands;

use FlowCatalyst\Definition\DefinitionRepository;
use FlowCatalyst\Sync\DefinitionSynchronizer;
use FlowCatalyst\Sync\SyncDefinitionSet;
use FlowCatalyst\Sync\SyncOptions;
use FlowCatalyst\Sync\SyncResult;
use Illuminate\Console\Command;

/**
 * Sync FlowCatalyst definitions to the platform.
 *
 * This command reads from the cached definitions (created by flowcatalyst:scan)
 * and syncs them to the platform. For programmatic sync without attributes,
 * use the DefinitionSynchronizer service directly.
 */
class SyncDefinitionsCommand extends Command
{
    protected $signature = 'flowcatalyst:sync
                            {--app= : Application code to sync (defaults to config value)}
                            {--roles : Sync only roles}
                            {--event-types : Sync only event types}
                            {--subscriptions : Sync only subscriptions}
                            {--dispatch-pools : Sync only dispatch pools}
                            {--principals : Sync only principals}
                            {--processes : Sync only processes}
                            {--scheduled-jobs : Sync only scheduled jobs}
                            {--openapi : Publish only the attached OpenAPI document}
                            {--openapi-file= : Path to a JSON/YAML OpenAPI file to attach before sync}
                            {--remove-unlisted : Remove definitions not in local cache}
                            {--no-spatie : Skip seeding roles/permissions into the local Spatie tables}
                            {--dry-run : Show what would be synced without actually syncing}';

    protected $description = 'Sync FlowCatalyst definitions to the platform';

    public function handle(
        DefinitionRepository $repository,
        DefinitionSynchronizer $synchronizer
    ): int {
        if (!$repository->cacheExists()) {
            $this->error('No cached definitions found. Run `php artisan flowcatalyst:scan` first.');
            return Command::FAILURE;
        }

        $appCode = $this->option('app') ?: config('flowcatalyst.application_code');
        if (!$appCode) {
            $this->error('Application code not configured. Either:');
            $this->error('  - Set FLOWCATALYST_APP_CODE in your .env file');
            $this->error('  - Use the --app option: php artisan flowcatalyst:sync --app=my-app');
            return Command::FAILURE;
        }

        $dryRun = $this->option('dry-run');
        $removeUnlisted = $this->option('remove-unlisted');

        if ($dryRun) {
            $this->warn('DRY RUN - No changes will be made');
            $this->newLine();
        }

        // Build options from command flags
        $syncAll = !$this->option('roles')
            && !$this->option('event-types')
            && !$this->option('subscriptions')
            && !$this->option('dispatch-pools')
            && !$this->option('principals')
            && !$this->option('processes')
            && !$this->option('scheduled-jobs')
            && !$this->option('openapi');
        $options = new SyncOptions(
            removeUnlisted: $removeUnlisted,
            syncRoles: $syncAll || $this->option('roles'),
            syncEventTypes: $syncAll || $this->option('event-types'),
            syncSubscriptions: $syncAll || $this->option('subscriptions'),
            syncDispatchPools: $syncAll || $this->option('dispatch-pools'),
            syncPrincipals: $syncAll || $this->option('principals'),
            syncProcesses: $syncAll || $this->option('processes'),
            syncScheduledJobs: $syncAll || $this->option('scheduled-jobs'),
            syncOpenapi: $syncAll || $this->option('openapi'),
        );

        // Build definition set from cached definitions
        $scannedData = $repository->all();
        $definitions = SyncDefinitionSet::fromScannedDefinitions($appCode, $scannedData->toArray());

        // Attach an OpenAPI document when an explicit file is given
        $openapiFile = $this->option('openapi-file');
        if (is_string($openapiFile) && $openapiFile !== '') {
            if (!is_file($openapiFile)) {
                $this->error("OpenAPI file not found: {$openapiFile}");
                return Command::FAILURE;
            }
            $raw = file_get_contents($openapiFile);
            if ($raw === false) {
                $this->error("Failed to read OpenAPI file: {$openapiFile}");
                return Command::FAILURE;
            }
            $decoded = json_decode($raw, true);
            if (!is_array($decoded)) {
                $this->error("OpenAPI file is not valid JSON: {$openapiFile}");
                return Command::FAILURE;
            }
            $definitions = $definitions->withOpenapiSpec($decoded);
        }

        if ($definitions->isEmpty()) {
            $this->info('No definitions to sync.');
            return Command::SUCCESS;
        }

        // Show what would be synced in dry-run mode
        if ($dryRun) {
            $this->showDryRunOutput($definitions, $options);
            return Command::SUCCESS;
        }

        // Perform the sync
        $this->info("Syncing definitions to application: {$appCode}");
        $result = $synchronizer->sync($definitions, $options);

        // Display results
        $this->displayResults($result);

        // Mirror roles + permissions into the local Spatie tables (so the app's
        // authorization model matches what it just pushed). FlowCatalyst has no
        // standalone permission entity, so permissions reach the platform via
        // the roles above; this step keeps the LOCAL side in step.
        $this->seedSpatie($definitions);

        return $result->hasErrors() ? Command::FAILURE : Command::SUCCESS;
    }

    /**
     * Seed roles + permissions into the local Spatie tables.
     */
    private function seedSpatie(SyncDefinitionSet $definitions): void
    {
        if ($this->option('no-spatie') || !config('flowcatalyst.definitions.seed_spatie', true)) {
            return;
        }

        $guard = (string) config('flowcatalyst.oidc.roles_guard', 'web');
        $seeder = new \FlowCatalyst\Sync\SpatieSeeder($guard);

        if (!$seeder->isAvailable()) {
            return; // spatie/laravel-permission not installed — nothing to seed.
        }

        try {
            $counts = $seeder->seed($definitions);
            $this->newLine();
            $this->info(sprintf(
                'Seeded into Spatie [%s]: %d role(s), %d permission(s).',
                $guard,
                $counts['roles'],
                $counts['permissions'],
            ));
        } catch (\Throwable $e) {
            $this->newLine();
            $this->warn('Spatie seeding skipped: ' . $e->getMessage());
        }
    }

    /**
     * Show what would be synced without actually syncing.
     */
    private function showDryRunOutput(SyncDefinitionSet $definitions, SyncOptions $options): void
    {
        if ($options->syncRoles && $definitions->hasRoles()) {
            $this->info('Roles to sync:');
            foreach ($definitions->getRoles() as $role) {
                $perms = $role['permissions'] ?? [];
                $suffix = $perms === [] ? '' : '  (' . implode(', ', $perms) . ')';
                $this->line("  - {$role['name']}{$suffix}");
            }
            $this->newLine();
        }

        if ($definitions->hasPermissions()) {
            $this->info('Permissions (seeded locally; reach FlowCatalyst via the roles that grant them):');
            foreach ($definitions->getPermissions() as $perm) {
                $name = is_array($perm) ? ($perm['permission'] ?? '') : (string) $perm;
                $this->line("  - {$name}");
            }
            $this->newLine();
        }

        if ($options->syncEventTypes && $definitions->hasEventTypes()) {
            $this->info('Event types to sync:');
            foreach ($definitions->getEventTypes() as $et) {
                $this->line("  - {$et['code']}");
            }
            $this->newLine();
        }

        if ($options->syncSubscriptions && $definitions->hasSubscriptions()) {
            $this->info('Subscriptions to sync:');
            foreach ($definitions->getSubscriptions() as $sub) {
                $this->line("  - {$sub['code']}");
            }
            $this->newLine();
        }

        if ($options->syncDispatchPools && $definitions->hasDispatchPools()) {
            $this->info('Dispatch pools to sync:');
            foreach ($definitions->getDispatchPools() as $pool) {
                $this->line("  - {$pool['code']}");
            }
            $this->newLine();
        }

        if ($options->syncProcesses && $definitions->hasProcesses()) {
            $this->info('Processes to sync:');
            foreach ($definitions->getProcesses() as $process) {
                $code = $process['code']
                    ?? sprintf(
                        '%s:%s:%s',
                        $definitions->applicationCode,
                        $process['subdomain'] ?? '',
                        $process['processName'] ?? '',
                    );
                $this->line("  - {$code}");
            }
            $this->newLine();
        }

        if ($options->syncScheduledJobs && $definitions->hasScheduledJobs()) {
            $this->info('Scheduled jobs to sync:');
            foreach ($definitions->getScheduledJobs() as $job) {
                $crons = implode(' | ', (array) ($job['crons'] ?? []));
                $this->line("  - {$job['code']}  [{$crons}]");
            }
            $this->newLine();
        }

        if ($options->syncOpenapi && $definitions->hasOpenapiSpec()) {
            $this->info('OpenAPI document attached — will publish on sync.');
            $this->newLine();
        }
    }

    /**
     * Display sync results.
     */
    private function displayResults(SyncResult $result): void
    {
        // Show errors if any
        if ($result->hasErrors()) {
            $this->newLine();
            $this->error('Sync completed with errors:');
            foreach ($result->getErrors() as $type => $error) {
                $this->error("  {$type}: {$error}");
            }
        }

        // Summary table
        $this->newLine();
        $this->info('Sync Summary:');
        $this->table(
            ['Type', 'Created', 'Updated', 'Deleted'],
            [
                [
                    'Roles',
                    $result->roles['created'] ?? 0,
                    $result->roles['updated'] ?? 0,
                    $result->roles['deleted'] ?? 0,
                ],
                [
                    'Event Types',
                    $result->eventTypes['created'] ?? 0,
                    $result->eventTypes['updated'] ?? 0,
                    $result->eventTypes['deleted'] ?? 0,
                ],
                [
                    'Subscriptions',
                    $result->subscriptions['created'] ?? 0,
                    $result->subscriptions['updated'] ?? 0,
                    $result->subscriptions['deleted'] ?? 0,
                ],
                [
                    'Dispatch Pools',
                    $result->dispatchPools['created'] ?? 0,
                    $result->dispatchPools['updated'] ?? 0,
                    $result->dispatchPools['deleted'] ?? 0,
                ],
                [
                    'Processes',
                    $result->processes['created'] ?? 0,
                    $result->processes['updated'] ?? 0,
                    $result->processes['deleted'] ?? 0,
                ],
                [
                    'Principals',
                    $result->principals['created'] ?? 0,
                    $result->principals['updated'] ?? 0,
                    $result->principals['deleted'] ?? 0,
                ],
                [
                    'Scheduled Jobs',
                    $result->scheduledJobs['created'] ?? 0,
                    $result->scheduledJobs['updated'] ?? 0,
                    $result->scheduledJobs['deleted'] ?? 0,
                ],
                [
                    'OpenAPI'
                        . (isset($result->openapi['version']) && $result->openapi['version'] !== ''
                            ? ' (' . $result->openapi['version'] . ')'
                            : ''),
                    $result->openapi['created'] ?? 0,
                    $result->openapi['updated'] ?? 0,
                    $result->openapi['deleted'] ?? 0,
                ],
            ]
        );
    }
}
