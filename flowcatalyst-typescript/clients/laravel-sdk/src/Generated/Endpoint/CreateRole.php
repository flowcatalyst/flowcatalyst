<?php

namespace FlowCatalyst\Generated\Endpoint;

class CreateRole extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * Creates a new role with source=DATABASE. Role name is auto-prefixed with application code.
     * @param null|\FlowCatalyst\Generated\Model\CreateRoleRequest1 $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\CreateRoleRequest1 $requestBody = null)
    {
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return '/api/admin/roles';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\CreateRoleRequest1) {
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
     * @throws \FlowCatalyst\Generated\Exception\CreateRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\CreateRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\CreateRoleConflictException
     *
     * @return null|\FlowCatalyst\Generated\Model\RoleDto1
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\RoleDto1', 'json');
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\CreateRoleBadRequestException($response);
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\CreateRoleNotFoundException($response);
        }
        if (409 === $status) {
            throw new \FlowCatalyst\Generated\Exception\CreateRoleConflictException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}