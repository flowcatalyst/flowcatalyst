<?php

namespace FlowCatalyst\Generated\Model;

class SyncPushKeysRequest
{
    /**
     * @var array
     */
    protected $initialized = [];
    public function isInitialized($property): bool
    {
        return array_key_exists($property, $this->initialized);
    }
    /**
     * At most 4 public Ed25519 keys: the current and the previous during a rotation
     *
     * @var list<SyncPushKeyRequest>|null
     */
    protected $keys;
    /**
     * At most 4 public Ed25519 keys: the current and the previous during a rotation
     *
     * @return list<SyncPushKeyRequest>|null
     */
    public function getKeys(): ?array
    {
        return $this->keys;
    }
    /**
     * At most 4 public Ed25519 keys: the current and the previous during a rotation
     *
     * @param list<SyncPushKeyRequest>|null $keys
     *
     * @return self
     */
    public function setKeys(?array $keys): self
    {
        $this->initialized['keys'] = true;
        $this->keys = $keys;
        return $this;
    }
}