<?php

namespace FlowCatalyst\Generated\Endpoint;

class CreateApplicationRole extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $appCode;
    /**
     * Creates a single role for the application with source=SDK.
     * @param string $appCode
     * @param null|\FlowCatalyst\Generated\Model\CreateRoleRequest2 $requestBody
     */
    public function __construct(string $appCode, ?\FlowCatalyst\Generated\Model\CreateRoleRequest2 $requestBody = null)
    {
        $this->appCode = $appCode;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{appCode}'], [$this->appCode], '/api/applications/{appCode}/roles');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\CreateRoleRequest2) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\CreateApplicationRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\CreateApplicationRoleConflictException
     * @throws \FlowCatalyst\Generated\Exception\CreateApplicationRoleBadRequestException
     *
     * @return null|\FlowCatalyst\Generated\Model\RoleDto2
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\RoleDto2', 'json');
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\CreateApplicationRoleNotFoundException($response);
        }
        if (409 === $status) {
            throw new \FlowCatalyst\Generated\Exception\CreateApplicationRoleConflictException($response);
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\CreateApplicationRoleBadRequestException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}