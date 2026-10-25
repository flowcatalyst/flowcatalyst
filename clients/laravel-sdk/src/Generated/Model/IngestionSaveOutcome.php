<?php

namespace FlowCatalyst\Generated\Model;

class IngestionSaveOutcome
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
     * The definition is in a version that is not live; someone who holds platform:ingestion:ingestion:promote makes it live
     *
     * @var bool|null
     */
    protected $awaitingPromote;
    /**
     * @var string|null
     */
    protected $message;
    /**
     * UNCHANGED: the live version already says it, nothing written. SAVED: saved as a new version and made live. MADE_LIVE: a saved version that was not live already says it and was made live, nothing new saved. DRAFT: saved as a new version that is not live. DRAFT_EXISTS: a saved version that is not live already says it, nothing saved
     *
     * @var string|null
     */
    protected $outcome;
    /**
     * The live version (UNCHANGED, SAVED, MADE_LIVE) or the draft (DRAFT, DRAFT_EXISTS)
     *
     * @var int|null
     */
    protected $version;
    /**
     * The definition is in a version that is not live; someone who holds platform:ingestion:ingestion:promote makes it live
     *
     * @return bool|null
     */
    public function getAwaitingPromote(): ?bool
    {
        return $this->awaitingPromote;
    }
    /**
     * The definition is in a version that is not live; someone who holds platform:ingestion:ingestion:promote makes it live
     *
     * @param bool|null $awaitingPromote
     *
     * @return self
     */
    public function setAwaitingPromote(?bool $awaitingPromote): self
    {
        $this->initialized['awaitingPromote'] = true;
        $this->awaitingPromote = $awaitingPromote;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getMessage(): ?string
    {
        return $this->message;
    }
    /**
     * @param string|null $message
     *
     * @return self
     */
    public function setMessage(?string $message): self
    {
        $this->initialized['message'] = true;
        $this->message = $message;
        return $this;
    }
    /**
     * UNCHANGED: the live version already says it, nothing written. SAVED: saved as a new version and made live. MADE_LIVE: a saved version that was not live already says it and was made live, nothing new saved. DRAFT: saved as a new version that is not live. DRAFT_EXISTS: a saved version that is not live already says it, nothing saved
     *
     * @return string|null
     */
    public function getOutcome(): ?string
    {
        return $this->outcome;
    }
    /**
     * UNCHANGED: the live version already says it, nothing written. SAVED: saved as a new version and made live. MADE_LIVE: a saved version that was not live already says it and was made live, nothing new saved. DRAFT: saved as a new version that is not live. DRAFT_EXISTS: a saved version that is not live already says it, nothing saved
     *
     * @param string|null $outcome
     *
     * @return self
     */
    public function setOutcome(?string $outcome): self
    {
        $this->initialized['outcome'] = true;
        $this->outcome = $outcome;
        return $this;
    }
    /**
     * The live version (UNCHANGED, SAVED, MADE_LIVE) or the draft (DRAFT, DRAFT_EXISTS)
     *
     * @return int|null
     */
    public function getVersion(): ?int
    {
        return $this->version;
    }
    /**
     * The live version (UNCHANGED, SAVED, MADE_LIVE) or the draft (DRAFT, DRAFT_EXISTS)
     *
     * @param int|null $version
     *
     * @return self
     */
    public function setVersion(?int $version): self
    {
        $this->initialized['version'] = true;
        $this->version = $version;
        return $this;
    }
}