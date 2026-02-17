<?php

namespace FlowCatalyst\Generated\Endpoint;

class DeleteApiAdminConfigAccessByAppCodeByRoleCode extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $appCode;
    protected $roleCode;
    /**
     * @param string $appCode
     * @param string $roleCode
     */
    public function __construct(string $appCode, string $roleCode)
    {
        $this->appCode = $appCode;
        $this->roleCode = $roleCode;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'DELETE';
    }
    public function getUri(): string
    {
        return str_replace(['{appCode}', '{roleCode}'], [$this->appCode, $this->roleCode], '/api/admin/config-access/{appCode}/{roleCode}');
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
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminConfigAccessByAppCodeByRoleCodeNotFoundException
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
            throw new \FlowCatalyst\Generated\Exception\DeleteApiAdminConfigAccessByAppCodeByRoleCodeNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodeDeleteResponse404', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}