<?php

namespace FlowCatalyst\Generated\Endpoint;

class SdkAddPrincipalRole extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    protected $roleName;
    /**
     * @param string $id
     * @param string $roleName
     */
    public function __construct(string $id, string $roleName)
    {
        $this->id = $id;
        $this->roleName = $roleName;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{id}', '{roleName}'], [$this->id, $this->roleName], '/api/sdk/principals/{id}/roles/{roleName}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\SdkAddPrincipalRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\SdkAddPrincipalRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkAddPrincipalRoleUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkAddPrincipalRoleForbiddenException
     *
     * @return null|\FlowCatalyst\Generated\Model\RoleAssignmentDto
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\RoleAssignmentDto', 'json');
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkAddPrincipalRoleBadRequestException($response);
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkAddPrincipalRoleNotFoundException($response);
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkAddPrincipalRoleUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkAddPrincipalRoleForbiddenException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}