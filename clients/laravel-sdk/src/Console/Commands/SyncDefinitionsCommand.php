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
                            {--remove-unlisted : Remove definitions not in local cache}
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
        $syncAll = !$this->option('roles') && !$this->option('event-types') && !$this->option('subscriptions') && !$this->option('dispatch-pools');
        $options = new SyncOptions(
            removeUnlisted: $removeUnlisted,
            syncRoles: $syncAll || $this->option('roles'),
            syncEventTypes: $syncAll || $this->option('event-types'),
            syncSubscriptions: $syncAll || $this->option('subscriptions'),
            syncDispatchPools: $syncAll || $this->option('dispatch-pools'),
        );

        // Build definition set from cached definitions
        $scannedData = $repository->all();
        $definitions = SyncDefinitionSet::fromScannedDefinitions($appCode, $scannedData->toArray());

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

        return $result->hasErrors() ? Command::FAILURE : Command::SUCCESS;
    }

    /**
     * Show what would be synced without actually syncing.
     */
    private function showDryRunOutput(SyncDefinitionSet $definitions, SyncOptions $options): void
    {
        if ($options->syncRoles && $definitions->hasRoles()) {
            $this->info('Roles to sync:');
            foreach ($definitions->getRoles() as $role) {
                $this->line("  - {$role['name']}");
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
            ]
        );
    }
}
