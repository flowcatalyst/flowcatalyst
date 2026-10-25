<?php

namespace FlowCatalyst\Generated\Model;

class SetFile
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
     * @var string|null
     */
    protected $content;
    /**
     * @var string|null
     */
    protected $path;
    /**
     * @return string|null
     */
    public function getContent(): ?string
    {
        return $this->content;
    }
    /**
     * @param string|null $content
     *
     * @return self
     */
    public function setContent(?string $content): self
    {
        $this->initialized['content'] = true;
        $this->content = $content;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getPath(): ?string
    {
        return $this->path;
    }
    /**
     * @param string|null $path
     *
     * @return self
     */
    public function setPath(?string $path): self
    {
        $this->initialized['path'] = true;
        $this->path = $path;
        return $this;
    }
}