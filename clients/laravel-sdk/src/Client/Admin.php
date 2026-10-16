<?php

declare(strict_types=1);

namespace FlowCatalyst\Client;

use FlowCatalyst\Client\Resources\Applications;
use FlowCatalyst\Client\Resources\Clients;
use FlowCatalyst\Client\Resources\Principals;
use FlowCatalyst\Client\Resources\Router;

/**
 * Administrative surface of the platform API: user, client and application
 * management and router checks. Obtain it with `$client->admin()`.
 *
 * These are the same resource classes as `$client->principals()`,
 * `->clients()`, `->applications()` and `->router()`; calls behave
 * identically, but going through here does not emit the deprecation notices
 * that the old accessors now raise for the methods that live under Admin.
 * An application's service account normally cannot call most of these routes;
 * they exist for operator tooling and for apps that delegate user management.
 */
class Admin
{
    private ?Principals $principals = null;
    private ?Clients $clients = null;
    private ?Applications $applications = null;
    private ?Router $router = null;

    public function __construct(private readonly FlowCatalystClient $client) {}

    /**
     * Principals: list, get, findByEmail, createUser, resetPassword, update,
     * activate/deactivate, getRoles/addRole/removeRole/setRoles,
     * client access grants, setClientAssociation. (`sync` and `syncUsers`
     * are also reachable here, and are not deprecated on the old accessor.)
     */
    public function principals(): Principals
    {
        return $this->principals ??= (new Principals($this->client))->viaAdmin();
    }

    /**
     * Clients: list, get, getByIdentifier (and Raw variants), search, create,
     * update, activate/suspend/deactivate, addNote, application enablement.
     */
    public function clients(): Clients
    {
        return $this->clients ??= (new Clients($this->client))->viaAdmin();
    }

    /**
     * Applications: list, create, update, delete, activate/deactivate,
     * service accounts, listClients, per-client config and enablement.
     */
    public function applications(): Applications
    {
        return $this->applications ??= (new Applications($this->client))->viaAdmin();
    }

    /**
     * Router in-pipeline checks (`inPipeline`, `inPipelineBatch`).
     */
    public function router(): Router
    {
        return $this->router ??= (new Router($this->client))->viaAdmin();
    }
}
