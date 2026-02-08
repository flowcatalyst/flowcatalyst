<?php

namespace FlowCatalyst\Generated\Endpoint;

class SdkRemovePrincipalRole extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
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
        return str_replace(['{id}', '{roleName}'], [$this->id, $this->roleName], '/api/sdk/principals/{id}/roles/{roleName}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\SdkRemovePrincipalRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkRemovePrincipalRoleUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkRemovePrincipalRoleForbiddenException
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
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkRemovePrincipalRoleNotFoundException($response);
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkRemovePrincipalRoleUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkRemovePrincipalRoleForbiddenException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}