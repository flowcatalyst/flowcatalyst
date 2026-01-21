<?php

declare(strict_types=1);

namespace FlowCatalyst\Facades;

use FlowCatalyst\Outbox\Contracts\OutboxDriver;
use FlowCatalyst\Outbox\DTOs\CreateDispatchJobDto;
use FlowCatalyst\Outbox\DTOs\CreateEventDto;
use FlowCatalyst\Outbox\OutboxManager;
use Illuminate\Support\Facades\Facade;

/**
 * @method static string createEvent(CreateEventDto $event)
 * @method static string createDispatchJob(CreateDispatchJobDto $job)
 * @method static string[] createEvents(array $events)
 * @method static string[] createDispatchJobs(array $jobs)
 * @method static OutboxDriver driver()
 *
 * @see OutboxManager
 */
class Outbox extends Facade
{
    /**
     * Get the registered name of the component.
     */
    protected static function getFacadeAccessor(): string
    {
        return OutboxManager::class;
    }
}
