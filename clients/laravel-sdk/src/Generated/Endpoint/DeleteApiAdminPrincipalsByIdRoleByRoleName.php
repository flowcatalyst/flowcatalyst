<?php

namespace FlowCatalyst\Generated\Endpoint;

class DeleteApiAdminPrincipalsByIdRoleByRoleName extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
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
        return 'DELETE';
    }
    public function getUri(): string
    {
        return str_replace(['{id}', '{roleName}'], [$this->id, $this->roleName], '/api/admin/principals/{id}/roles/{roleName}');
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
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminPrincipalsByIdRoleByRoleNameNotFoundException
     *
     * @return null
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (204 === $status) {
            return null;
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\DeleteApiAdminPrincipalsByIdRoleByRoleNameNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesRoleNameDeleteResponse404', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}