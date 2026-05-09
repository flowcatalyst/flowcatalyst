<?php

namespace FlowCatalyst\Generated\Model;

class LoginRequest extends \ArrayObject
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
     * Email address
     *
     * @var string|null
     */
    protected $email;
    /**
     * Password
     *
     * @var string|null
     */
    protected $password;
    /**
     * Remember me (extends session duration)
     *
     * @var bool|null
     */
    protected $rememberMe;
    /**
     * Email address
     *
     * @return string|null
     */
    public function getEmail(): ?string
    {
        return $this->email;
    }
    /**
     * Email address
     *
     * @param string|null $email
     *
     * @return self
     */
    public function setEmail(?string $email): self
    {
        $this->initialized['email'] = true;
        $this->email = $email;
        return $this;
    }
    /**
     * Password
     *
     * @return string|null
     */
    public function getPassword(): ?string
    {
        return $this->password;
    }
    /**
     * Password
     *
     * @param string|null $password
     *
     * @return self
     */
    public function setPassword(?string $password): self
    {
        $this->initialized['password'] = true;
        $this->password = $password;
        return $this;
    }
    /**
     * Remember me (extends session duration)
     *
     * @return bool|null
     */
    public function getRememberMe(): ?bool
    {
        return $this->rememberMe;
    }
    /**
     * Remember me (extends session duration)
     *
     * @param bool|null $rememberMe
     *
     * @return self
     */
    public function setRememberMe(?bool $rememberMe): self
    {
        $this->initialized['rememberMe'] = true;
        $this->rememberMe = $rememberMe;
        return $this;
    }
}