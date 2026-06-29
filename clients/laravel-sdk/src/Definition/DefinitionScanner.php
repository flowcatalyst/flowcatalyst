<?php

declare(strict_types=1);

namespace FlowCatalyst\Definition;

use FlowCatalyst\Attributes\AsDispatchPool;
use FlowCatalyst\Attributes\AsEventType;
use FlowCatalyst\Attributes\AsPermission;
use FlowCatalyst\Attributes\AsProcess;
use FlowCatalyst\Attributes\AsRole;
use FlowCatalyst\Attributes\AsScheduledJob;
use FlowCatalyst\Attributes\AsSubscription;
use Illuminate\Support\Facades\File;
use ReflectionClass;
use Symfony\Component\Finder\Finder;

/**
 * Scans directories for FlowCatalyst attribute definitions.
 */
class DefinitionScanner
{
    /**
     * Scan configured paths for FlowCatalyst definitions.
     *
     * @param string[] $paths Directories to scan
     * @return ScannedDefinitions
     */
    public function scan(array $paths, ?string $applicationCode = null): ScannedDefinitions
    {
        // App code used to resolve #[AsPermission] segments + role permission
        // class-refs. Falls back to the configured application code.
        $applicationCode ??= (function () {
            $code = function_exists('config') ? config('flowcatalyst.application_code') : null;
            return is_string($code) && $code !== '' ? $code : null;
        })();

        $roles = [];
        $permissions = [];
        $eventTypes = [];
        $subscriptions = [];
        $dispatchPools = [];
        $processes = [];
        $scheduledJobs = [];

        foreach ($paths as $path) {
            if (!File::isDirectory($path)) {
                continue;
            }

            $finder = new Finder();
            $finder->files()->in($path)->name('*.php');

            foreach ($finder as $file) {
                $classes = $this->getClassesFromFile($file->getRealPath());

                foreach ($classes as $className) {
                    $this->processClass($className, $applicationCode, $roles, $permissions, $eventTypes, $subscriptions, $dispatchPools, $processes, $scheduledJobs);
                }
            }
        }

        return new ScannedDefinitions(
            roles: $roles,
            permissions: $permissions,
            eventTypes: $eventTypes,
            subscriptions: $subscriptions,
            dispatchPools: $dispatchPools,
            processes: $processes,
            scheduledJobs: $scheduledJobs,
        );
    }

    /**
     * Extract class names from a PHP file.
     *
     * @return string[]
     */
    private function getClassesFromFile(string $filepath): array
    {
        $content = file_get_contents($filepath);
        if ($content === false) {
            return [];
        }

        $classes = [];
        $namespace = '';

        // Extract namespace
        if (preg_match('/namespace\s+([^;]+);/', $content, $matches)) {
            $namespace = $matches[1];
        }

        // Extract class names
        if (preg_match_all('/^class\s+(\w+)/m', $content, $matches)) {
            foreach ($matches[1] as $className) {
                $fullClassName = $namespace ? "{$namespace}\\{$className}" : $className;
                if (class_exists($fullClassName)) {
                    $classes[] = $fullClassName;
                }
            }
        }

        return $classes;
    }

    /**
     * Process a class for FlowCatalyst attributes.
     *
     * @param array<array<string, mixed>> $roles
     * @param array<array<string, mixed>> $eventTypes
     * @param array<array<string, mixed>> $subscriptions
     * @param array<array<string, mixed>> $dispatchPools
     * @param array<array<string, mixed>> $processes
     * @param array<array<string, mixed>> $scheduledJobs
     */
    private function processClass(
        string $className,
        ?string $applicationCode,
        array &$roles,
        array &$permissions,
        array &$eventTypes,
        array &$subscriptions,
        array &$dispatchPools,
        array &$processes,
        array &$scheduledJobs
    ): void {
        try {
            $reflection = new ReflectionClass($className);
        } catch (\ReflectionException) {
            return;
        }

        // Check for AsPermission attribute (standalone permission definitions).
        $permissionAttributes = $reflection->getAttributes(AsPermission::class);
        foreach ($permissionAttributes as $attribute) {
            /** @var AsPermission $instance */
            $instance = $attribute->newInstance();
            $permissions[] = array_merge($instance->toArray($applicationCode), [
                '_class' => $className,
            ]);
        }

        // Check for AsRole attribute
        $roleAttributes = $reflection->getAttributes(AsRole::class);
        foreach ($roleAttributes as $attribute) {
            /** @var AsRole $instance */
            $instance = $attribute->newInstance();
            $roles[] = array_merge($instance->toArray($applicationCode), [
                '_class' => $className,
            ]);
        }

        // Check for AsEventType attribute
        $eventTypeAttributes = $reflection->getAttributes(AsEventType::class);
        foreach ($eventTypeAttributes as $attribute) {
            /** @var AsEventType $instance */
            $instance = $attribute->newInstance();
            $eventTypes[] = array_merge($instance->toArray(), [
                '_class' => $className,
            ]);
        }

        // Check for AsSubscription attribute
        $subscriptionAttributes = $reflection->getAttributes(AsSubscription::class);
        foreach ($subscriptionAttributes as $attribute) {
            /** @var AsSubscription $instance */
            $instance = $attribute->newInstance();
            $subscriptions[] = array_merge($instance->toArray(), [
                '_class' => $className,
            ]);
        }

        // Check for AsDispatchPool attribute
        $dispatchPoolAttributes = $reflection->getAttributes(AsDispatchPool::class);
        foreach ($dispatchPoolAttributes as $attribute) {
            /** @var AsDispatchPool $instance */
            $instance = $attribute->newInstance();
            $dispatchPools[] = array_merge($instance->toArray(), [
                '_class' => $className,
            ]);
        }

        // Check for AsProcess attribute
        $processAttributes = $reflection->getAttributes(AsProcess::class);
        foreach ($processAttributes as $attribute) {
            /** @var AsProcess $instance */
            $instance = $attribute->newInstance();
            $processes[] = array_merge($instance->toArray(), [
                '_class' => $className,
            ]);
        }

        // Check for AsScheduledJob attribute
        $scheduledJobAttributes = $reflection->getAttributes(AsScheduledJob::class);
        foreach ($scheduledJobAttributes as $attribute) {
            /** @var AsScheduledJob $instance */
            $instance = $attribute->newInstance();
            $scheduledJobs[] = array_merge($instance->toArray(), [
                '_class' => $className,
            ]);
        }
    }
}
