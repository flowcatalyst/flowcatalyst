<?php

namespace FlowCatalyst\Generated\Endpoint;

class UpdateRole extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $roleName;
    /**
     * Updates a role. CODE-sourced roles can only have clientManaged updated.
     * @param string $roleName
     * @param null|\FlowCatalyst\Generated\Model\UpdateRoleRequest1 $requestBody
     */
    public function __construct(string $roleName, ?\FlowCatalyst\Generated\Model\UpdateRoleRequest1 $requestBody = null)
    {
        $this->roleName = $roleName;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{roleName}'], [$this->roleName], '/api/admin/roles/{roleName}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\UpdateRoleRequest1) {
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
     * @throws \FlowCatalyst\Generated\Exception\UpdateRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\UpdateRoleBadRequestException
     *
     * @return null|\FlowCatalyst\Generated\Model\RoleDto1
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\RoleDto1', 'json');
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\UpdateRoleNotFoundException($response);
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\UpdateRoleBadRequestException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}