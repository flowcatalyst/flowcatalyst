<?php

namespace FlowCatalyst\Generated\Model;

class InstallLinkWithTokenResponse
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
     * A URL to the JSON Schema for this object.
     *
     * @var string|null
     */
    protected $dollarSchema;
    /**
     * @var AppInstallLinkResponse|null
     */
    protected $link;
    /**
     * The token in the address. Shown once.
     *
     * @var string|null
     */
    protected $token;
    /**
     * The address to send: anyone holding it can download the channel's binaries. Shown once.
     *
     * @var string|null
     */
    protected $url;
    /**
     * A URL to the JSON Schema for this object.
     *
     * @return string|null
     */
    public function getDollarSchema(): ?string
    {
        return $this->dollarSchema;
    }
    /**
     * A URL to the JSON Schema for this object.
     *
     * @param string|null $dollarSchema
     *
     * @return self
     */
    public function setDollarSchema(?string $dollarSchema): self
    {
        $this->initialized['dollarSchema'] = true;
        $this->dollarSchema = $dollarSchema;
        return $this;
    }
    /**
     * @return AppInstallLinkResponse|null
     */
    public function getLink(): ?AppInstallLinkResponse
    {
        return $this->link;
    }
    /**
     * @param AppInstallLinkResponse|null $link
     *
     * @return self
     */
    public function setLink(?AppInstallLinkResponse $link): self
    {
        $this->initialized['link'] = true;
        $this->link = $link;
        return $this;
    }
    /**
     * The token in the address. Shown once.
     *
     * @return string|null
     */
    public function getToken(): ?string
    {
        return $this->token;
    }
    /**
     * The token in the address. Shown once.
     *
     * @param string|null $token
     *
     * @return self
     */
    public function setToken(?string $token): self
    {
        $this->initialized['token'] = true;
        $this->token = $token;
        return $this;
    }
    /**
     * The address to send: anyone holding it can download the channel's binaries. Shown once.
     *
     * @return string|null
     */
    public function getUrl(): ?string
    {
        return $this->url;
    }
    /**
     * The address to send: anyone holding it can download the channel's binaries. Shown once.
     *
     * @param string|null $url
     *
     * @return self
     */
    public function setUrl(?string $url): self
    {
        $this->initialized['url'] = true;
        $this->url = $url;
        return $this;
    }
}