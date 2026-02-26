<?php

declare(strict_types=1);

namespace FlowCatalyst\Definition;

use FlowCatalyst\Attributes\AsDispatchPool;
use FlowCatalyst\Attributes\AsEventType;
use FlowCatalyst\Attributes\AsRole;
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
    public function scan(array $paths): ScannedDefinitions
    {
        $roles = [];
        $eventTypes = [];
        $subscriptions = [];
        $dispatchPools = [];

        foreach ($paths as $path) {
            if (!File::isDirectory($path)) {
                continue;
            }

            $finder = new Finder();
            $finder->files()->in($path)->name('*.php');

            foreach ($finder as $file) {
                $classes = $this->getClassesFromFile($file->getRealPath());

                foreach ($classes as $className) {
                    $this->processClass($className, $roles, $eventTypes, $subscriptions, $dispatchPools);
                }
            }
        }

        return new ScannedDefinitions(
            roles: $roles,
            eventTypes: $eventTypes,
            subscriptions: $subscriptions,
            dispatchPools: $dispatchPools
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
     */
    private function processClass(
        string $className,
        array &$roles,
        array &$eventTypes,
        array &$subscriptions,
        array &$dispatchPools
    ): void {
        try {
            $reflection = new ReflectionClass($className);
        } catch (\ReflectionException) {
            return;
        }

        // Check for AsRole attribute
        $roleAttributes = $reflection->getAttributes(AsRole::class);
        foreach ($roleAttributes as $attribute) {
            /** @var AsRole $instance */
            $instance = $attribute->newInstance();
            $roles[] = array_merge($instance->toArray(), [
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
    }
}
